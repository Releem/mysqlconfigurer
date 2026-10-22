package utils

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	logging "github.com/google/logger"
)

type resultRepeater struct {
	body string
	err  error
}

func (r resultRepeater) ProcessMetrics(models.MetricContext, models.Metrics, models.ModeType) (string, error) {
	return r.body, r.err
}

func TestProcessRepeatersReturnsNilOnError(t *testing.T) {
	logger := logging.Init("process-repeaters", false, false, io.Discard)
	defer logger.Close()
	for _, body := range []string{"", "partial response"} {
		result, err := ProcessRepeaters(&models.Metrics{}, resultRepeater{body: body, err: context.DeadlineExceeded}, &config.Config{}, *logger, models.ModeType{})
		if result != nil || !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("ProcessRepeaters(%q) = %q, %v; want nil, deadline exceeded", body, result, err)
		}
	}
	result, err := ProcessRepeaters(&models.Metrics{}, resultRepeater{body: "Task"}, &config.Config{}, *logger, models.ModeType{})
	if err != nil || string(result) != "Task" {
		t.Errorf("successful response = %q, %v; want Task, nil", result, err)
	}
	result, err = ProcessRepeaters(nil, resultRepeater{}, &config.Config{}, *logger, models.ModeType{})
	if result != nil || err == nil {
		t.Errorf("nil metrics = %q, %v; want nil, error", result, err)
	}
}

func TestProcessRepeatersRejectsEmptyConfigurationResponses(t *testing.T) {
	logger := logging.Init("empty-configuration-response", false, false, io.Discard)
	defer logger.Close()
	for _, body := range []string{"", " \n\t"} {
		result, err := ProcessRepeaters(&models.Metrics{}, resultRepeater{body: body}, &config.Config{}, *logger, models.ModeType{Name: "Configurations", Type: "GetJson"})
		if result != nil || err == nil {
			t.Errorf("ProcessRepeaters(configuration %q) = %q, %v; want nil, error", body, result, err)
		}
	}
}

type panickingRepeater struct{}

func (panickingRepeater) ProcessMetrics(models.MetricContext, models.Metrics, models.ModeType) (string, error) {
	panic("response handling failed")
}

type panicReportTransport func(*http.Request) (*http.Response, error)

func (f panicReportTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

// A recovered repeater panic must fail the call; before, it returned (nil, nil)
// and a configuration download was reported as successful.
func TestProcessRepeatersReturnsErrorAfterRepeaterPanic(t *testing.T) {
	// Sequential: the panic report goes through the shared default transport.
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	reports := 0
	http.DefaultTransport = panicReportTransport(func(request *http.Request) (*http.Response, error) {
		reports++
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: request}, nil
	})
	logger := logging.Init("process-repeaters-panic", false, false, io.Discard)
	defer logger.Close()

	result, err := ProcessRepeaters(&models.Metrics{}, panickingRepeater{}, &config.Config{}, *logger, models.ModeType{Name: "Configurations", Type: "Get"})
	if result != nil || err == nil || !strings.Contains(err.Error(), "response handling failed") {
		t.Fatalf("ProcessRepeaters(panic) = %q, %v; want nil and the panic as an error", result, err)
	}
	if reports != 1 {
		t.Fatalf("panic reports = %d, want 1", reports)
	}
}
