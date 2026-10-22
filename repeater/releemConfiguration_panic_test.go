package repeater_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/repeater"
	"github.com/Releem/mysqlconfigurer/utils"
	logging "github.com/google/logger"
)

type panickingRequestTransport func(*http.Request) (*http.Response, error)

func (f panickingRequestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestProcessRepeatersReturnsRealRepeaterPanicsAsErrors(t *testing.T) {
	// Sequential: the request and panic report use the shared default transport.
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	reports := 0
	http.DefaultTransport = panickingRequestTransport(func(request *http.Request) (*http.Response, error) {
		if strings.HasSuffix(request.URL.Path, "/agent_errors_log") {
			reports++
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
				Request:    request,
			}, nil
		}
		panic("controlled request failure")
	})
	logger := logging.Init("real-repeater-panic", false, false, io.Discard)
	defer logger.Close()
	configuration := &config.Config{}
	actual := repeater.NewReleemConfigurationsRepeater(configuration, *logger)
	modes := []models.ModeType{
		{Name: "Task", Type: "Get"},
		{Name: "Task", Type: "Status"},
		{Name: "Metrics"},
		{Name: "Event", Type: "config_applied"},
		{Name: "Configurations", Type: "Get"},
	}
	for _, mode := range modes {
		t.Run(mode.Name+"/"+mode.Type, func(t *testing.T) {
			response, err := utils.ProcessRepeaters(&models.Metrics{}, actual, configuration, *logger, mode)
			if response != nil || err == nil || !strings.Contains(err.Error(), "controlled request failure") {
				t.Errorf("ProcessRepeaters(%s/%s panic) = %q, %v; want nil and the panic error", mode.Name, mode.Type, response, err)
			}
		})
	}
	if reports != len(modes) {
		t.Errorf("panic reports = %d, want %d (one per failed request)", reports, len(modes))
	}
}
