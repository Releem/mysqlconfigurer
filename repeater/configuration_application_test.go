package repeater

import (
	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	logging "github.com/google/logger"
	"io"
	"net/http"
	"strings"
	"testing"
)

type applicationTransport func(*http.Request) (*http.Response, error)

func (f applicationTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestSelectedConfigurationHeaders(t *testing.T) {
	previous := http.DefaultTransport
	defer func() { http.DefaultTransport = previous }()
	http.DefaultTransport = applicationTransport(func(r *http.Request) (*http.Response, error) {
		for key, want := range map[string]string{"X-Releem-Configuration-Status-Id": "12", "X-Releem-Apply-Mode": "dynamic", "Accept": "application/json"} {
			if got := r.Header.Get(key); got != want {
				t.Errorf("%s=%q", key, got)
			}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"max_connections":"100"}`)), Header: make(http.Header)}, nil
	})
	c := &config.Config{ReleemConfDir: t.TempDir()}
	logger := *logging.Init("transport-test", false, false, io.Discard)
	r := NewReleemConfigurationsRepeater(c, logger)
	_, err := r.ProcessMetrics(c, models.Metrics{}, models.ModeType{Name: "Configurations", Type: "GetJson", ConfigurationStatusID: 12, ApplyMode: "dynamic"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestOptionalSelectionHeaders(t *testing.T) {
	previous := http.DefaultTransport
	defer func() { http.DefaultTransport = previous }()
	for _, selection := range []models.ModeType{{ConfigurationStatusID: 12}, {}} {
		http.DefaultTransport = applicationTransport(func(r *http.Request) (*http.Response, error) {
			wantID := ""
			if selection.ConfigurationStatusID > 0 {
				wantID = "12"
			}
			if r.Header.Get("X-Releem-Configuration-Status-Id") != wantID || r.Header.Get("X-Releem-Configuration-Id") != "" || r.Header.Get("X-Releem-Task-Id") != "" {
				t.Fatalf("unexpected headers: %v", r.Header)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
		})
		c := &config.Config{ReleemConfDir: t.TempDir()}
		logger := *logging.Init("optional-selection", false, false, io.Discard)
		r := NewReleemConfigurationsRepeater(c, logger)
		selection.Name, selection.Type, selection.ApplyMode = "Configurations", "GetJson", "dynamic"
		if _, err := r.ProcessMetrics(c, models.Metrics{}, selection); err != nil {
			t.Fatal(err)
		}
	}
}
