package tasks

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/repeater"
	logging "github.com/google/logger"
)

type preparedConfigurationTransport func(*http.Request) (*http.Response, error)

func (f preparedConfigurationTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestDownloadConfigurationUsesPermanentFile(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	logger := logging.Init("configuration-file", false, false, io.Discard)
	defer logger.Close()
	const body = "max_connections=200\n"
	http.DefaultTransport = preparedConfigurationTransport(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("X-Releem-Configuration-Status-Id") != "42" {
			t.Errorf("request lost selected configuration: %v", request.Header)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	for _, dbType := range []string{"mysql", "postgresql"} {
		t.Run(dbType, func(t *testing.T) {
			configuration := &config.Config{ReleemConfDir: t.TempDir()}
			filename := "z_aiops_mysql.cnf"
			if dbType == "postgresql" {
				configuration.PgUser = "test"
				configuration.PgPassword = "test"
				filename = "z_aiops_postgresql.conf"
			}
			r := repeater.NewReleemConfigurationsRepeater(configuration, *logger)
			for _, mode := range []ApplyModeType{ApplyModeDynamic, ApplyModeFull} {
				code, status, output := DownloadConfigurationFile(&models.Metrics{}, r, configuration, *logger, mode, &models.Task{TypeID: 5, Details: `{"configuration_status_id":42}`})
				if code != 0 || status != 1 {
					t.Fatalf("download(%s) = %d/%d %s", mode, code, status, output)
				}
				data, err := os.ReadFile(filepath.Join(configuration.ReleemConfDir, filename))
				if err != nil || string(data) != body {
					t.Fatalf("configuration = %q, %v; want %q", data, err, body)
				}
				entries, err := os.ReadDir(configuration.ReleemConfDir)
				if err != nil || len(entries) != 1 || entries[0].Name() != filename {
					t.Fatalf("download created extra files: %v, %v", entries, err)
				}
			}
		})
	}
}

func TestFullApplyUsesPermanentConfiguration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell stub; Windows apply has offline PowerShell coverage")
	}
	logger := logging.Init("full-apply", false, false, io.Discard)
	defer logger.Close()
	directory := t.TempDir()
	script := "#!/bin/sh\ncp \"$TEST_CANONICAL_CONFIG\" \"$TEST_LIVE_CONFIG\" || exit 90\nexit \"$TEST_APPLY_EXIT\"\n"
	if err := os.WriteFile(filepath.Join(directory, "releem.sh"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(directory, "live.cnf")
	t.Setenv("TEST_LIVE_CONFIG", live)
	for _, taskType := range []int{0, 5, 8} {
		for _, exitCode := range []int{0, 3} {
			t.Run(fmt.Sprintf("type_%d/exit_%d", taskType, exitCode), func(t *testing.T) {
				t.Setenv("TEST_APPLY_EXIT", fmt.Sprint(exitCode))
				configuration := &config.Config{ReleemDir: directory, ReleemConfDir: t.TempDir()}
				path := filepath.Join(configuration.ReleemConfDir, "z_aiops_mysql.cnf")
				t.Setenv("TEST_CANONICAL_CONFIG", path)
				r := &preparedTaskRepeater{taskType: taskType}
				synctest.Test(t, func(t *testing.T) { ProcessTask(r, nil, *logger, configuration) })
				if len(r.statuses) != 2 || r.statuses[1].ExitCode != exitCode {
					t.Fatalf("task statuses = %+v, want final exit %d", r.statuses, exitCode)
				}
				for _, file := range []string{path, live} {
					body, err := os.ReadFile(file)
					if err != nil || string(body) != "selected configuration" {
						t.Fatalf("configuration(%s) = %q, %v", file, body, err)
					}
				}
				entries, err := os.ReadDir(configuration.ReleemConfDir)
				if err != nil || len(entries) != 1 {
					t.Fatalf("unexpected files = %v, %v", entries, err)
				}
			})
		}
	}
}

type preparedTaskRepeater struct {
	taskType int
	statuses []models.Task
}

func (r *preparedTaskRepeater) ProcessMetrics(context models.MetricContext, metrics models.Metrics, mode models.ModeType) (string, error) {
	if mode.Name == "Task" {
		if mode.Type == "Get" {
			body, err := json.Marshal(models.Task{ID: 1, TypeID: r.taskType, Details: `{"configuration_status_id":42}`})
			return string(body), err
		}
		r.statuses = append(r.statuses, metrics.ReleemAgent.Tasks)
		return "", nil
	}
	return "selected configuration", os.WriteFile(filepath.Join(context.GetReleemConfDir(), "z_aiops_mysql.cnf"), []byte("selected configuration"), 0644)
}

func TestDynamicApplyUsesPermanentConfiguration(t *testing.T) {
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	http.DefaultTransport = preparedConfigurationTransport(func(request *http.Request) (*http.Response, error) {
		body := "[mysqld]\nmax_connections=200\n"
		if request.Header.Get("Accept") == "application/json" {
			body = `{"max_connections":"200"}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	logger := logging.Init("dynamic-configuration", false, false, io.Discard)
	defer logger.Close()
	configuration := &config.Config{ReleemConfDir: t.TempDir()}
	r := repeater.NewReleemConfigurationsRepeater(configuration, *logger)
	metrics := &models.Metrics{}
	metrics.DB.Conf.Variables = models.MetricGroupValue{"max_connections": "200"}
	synctest.Test(t, func(t *testing.T) {
		code, status, output := applyDynamicConfigurationLocal(metrics, r, *logger, configuration, &models.Task{TypeID: 4}, func(command taskCommand, _ logging.Logger) (int, int, string) {
			if want := taskApplyAutomaticCommand(runtime.GOOS, configuration.ReleemDir, false); !reflect.DeepEqual(command, want) {
				t.Errorf("command = %+v, want %+v", command, want)
			}
			data, err := os.ReadFile(filepath.Join(configuration.ReleemConfDir, "z_aiops_mysql.cnf"))
			if err != nil || !strings.Contains(string(data), "max_connections=200") {
				t.Errorf("staged configuration = %q, %v", data, err)
			}
			return 0, 1, ""
		})
		if code != 0 || status != 1 {
			t.Fatalf("dynamic apply = %d/%d %s", code, status, output)
		}
	})
}
