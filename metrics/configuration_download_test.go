package metrics

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/repeater"
	logging "github.com/google/logger"
)

func TestConfigurationDownloadHandlesTermination(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX termination signals are not available on Windows")
	}
	if directory := os.Getenv("RELEEM_TEST_SIGNAL_DIRECTORY"); directory != "" {
		logger := logging.Init("signal-download", false, false, io.Discard)
		defer logger.Close()
		defer func() {
			if err := os.WriteFile(filepath.Join(directory, "cleanup-ran"), nil, 0600); err != nil {
				t.Error(err)
			}
		}()
		http.DefaultTransport = configurationDownloadTransport(func(request *http.Request) (*http.Response, error) {
			if err := os.WriteFile(filepath.Join(directory, "request-started"), nil, 0600); err != nil {
				return nil, err
			}
			<-request.Context().Done()
			return nil, request.Context().Err()
		})
		configuration := &config.Config{PgUser: "test", PgPassword: "test", ReleemConfDir: directory}
		r := repeater.NewReleemConfigurationsRepeater(configuration, *logger)
		err := RunWorker(nil, r, *logger, configuration, models.ModeType{Name: "Configurations", Type: "Get"})
		if err != nil {
			t.Errorf("RunWorker after SIGTERM = %v, want nil", err)
		}
		return
	}
	directory := t.TempDir()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestConfigurationDownloadHandlesTermination$")
	cmd.Env = append(os.Environ(), "RELEEM_TEST_SIGNAL_DIRECTORY="+directory)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(filepath.Join(directory, "request-started")); err == nil {
			break
		}
		select {
		case err := <-exited:
			t.Fatalf("download exited before request began: %v: %s", err, &output)
		case <-ctx.Done():
			cancel()
			<-exited
			t.Fatal("download request did not begin")
		case <-ticker.C:
		}
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-exited:
		if err != nil {
			t.Fatalf("interrupted download failed: %v: %s", err, &output)
		}
	case <-time.After(2 * time.Second):
		cancel()
		<-exited
		t.Fatalf("download ignored SIGTERM: %s", &output)
	}
	if _, err := os.Stat(filepath.Join(directory, "cleanup-ran")); err != nil {
		t.Errorf("interrupted download skipped caller cleanup: %v", err)
	}
}

func TestConfigurationDownloadReturnsAfterCleanup(t *testing.T) {
	if scenario := os.Getenv("RELEEM_TEST_DOWNLOAD_RESULT"); scenario != "" {
		logger := logging.Init("download-command", false, false, io.Discard)
		configuration := &config.Config{PgUser: "test", PgPassword: "test", ReleemConfDir: os.Getenv("RELEEM_TEST_DOWNLOAD_DIRECTORY")}
		http.DefaultTransport = configurationDownloadTransport(func(*http.Request) (*http.Response, error) {
			status, body := 200, "max_connections=200\n"
			if scenario == "http_failure" {
				status, body = 500, "temporary failure"
			} else if scenario == "empty_response" {
				body = ""
			}
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})
		r := repeater.NewReleemConfigurationsRepeater(configuration, *logger)
		gatherers := map[string][]models.MetricsGatherer{}
		if scenario == "collection_failure" {
			gatherers["default"] = []models.MetricsGatherer{failedDownloadGatherer{}}
		}
		defer logger.Close()
		defer func() {
			if err := os.WriteFile(filepath.Join(configuration.ReleemConfDir, "cleanup-ran"), []byte("done"), 0600); err != nil {
				t.Error(err)
			}
		}()
		err := RunWorker(gatherers, r, *logger, configuration, models.ModeType{Name: "Configurations", Type: "Get", ApplyMode: "full"})
		if (err == nil) != (scenario == "success") {
			t.Errorf("RunWorker(%s) = %v, want success=%t", scenario, err, scenario == "success")
		}
		return
	}
	for _, scenario := range []string{"http_failure", "empty_response", "collection_failure", "success"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestConfigurationDownloadReturnsAfterCleanup$")
			directory := t.TempDir()
			cmd.Env = append(os.Environ(), "RELEEM_TEST_DOWNLOAD_RESULT="+scenario, "RELEEM_TEST_DOWNLOAD_DIRECTORY="+directory)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("RunWorker(%s) did not return normally: %v: %s", scenario, err, output)
			}
			if _, err := os.Stat(filepath.Join(directory, "cleanup-ran")); err != nil {
				t.Fatalf("RunWorker(%s) skipped caller cleanup: %v", scenario, err)
			}
		})
	}
}

type failedDownloadGatherer struct{}

func (failedDownloadGatherer) GetMetrics(*models.Metrics) error {
	return errors.New("database unavailable")
}

type configurationDownloadTransport func(*http.Request) (*http.Response, error)

func (f configurationDownloadTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestConfigurationDownloadRejectsFailuresAndEmptyResponses(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	logger := logging.Init("configuration-download", false, false, io.Discard)
	defer logger.Close()
	for _, tc := range []struct {
		name   string
		status int
		body   string
		failed bool
	}{
		{"http_failure", 500, "temporary failure", true},
		{"empty_response", 200, "", true},
		{"success", 200, "[mysqld]\nmax_connections=200\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			http.DefaultTransport = configurationDownloadTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})
			configuration := &config.Config{ReleemConfDir: t.TempDir()}
			path := filepath.Join(configuration.ReleemConfDir, "z_aiops_mysql.cnf")
			if err := os.WriteFile(path, []byte("old configuration"), 0644); err != nil {
				t.Fatal(err)
			}
			r := repeater.NewReleemConfigurationsRepeater(configuration, *logger)
			err := collectConfiguration(nil, r, *logger, configuration, models.ModeType{Name: "Configurations", Type: "Get", ApplyMode: "full"})
			if (err != nil) != tc.failed {
				t.Errorf("configuration download error = %v, want failure=%t", err, tc.failed)
			}
			if !tc.failed {
				body, err := os.ReadFile(path)
				if err != nil || string(body) != tc.body {
					t.Errorf("downloaded configuration = %q, %v; want %q", body, err, tc.body)
				}
			}
		})
	}
}
