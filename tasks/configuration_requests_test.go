package tasks

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/repeater"
	logging "github.com/google/logger"
)

func TestRecommendationRequestPreservesErrorsAndRejectsPartialBody(t *testing.T) {
	wantErr := errors.New("connection interrupted")
	r := &downloadCapture{body: `{"max_connections":"200"}`, err: wantErr}
	mode := models.ModeType{Name: "Configurations", Type: "GetJson", ApplyMode: "dynamic", ConfigurationStatusID: 23}
	logger := logging.Init("recommendation-errors", false, false, io.Discard)
	defer logger.Close()
	body, err := requestConfigurationRecommendations(&models.Metrics{}, r, &config.Config{}, *logger, mode)
	if body != "" || !errors.Is(err, wantErr) || r.mode != mode {
		t.Fatalf("requestConfigurationRecommendations = %q, %v, mode %+v; want empty body, original error, mode %+v", body, err, r.mode, mode)
	}
}

func TestEmptyRecommendationResponseIsARequestFailure(t *testing.T) {
	logger := logging.Init("empty-recommendations", false, false, io.Discard)
	defer logger.Close()
	for _, response := range []string{"", " \n\t"} {
		body, err := requestConfigurationRecommendations(&models.Metrics{}, &downloadCapture{body: response}, &config.Config{}, *logger, models.ModeType{Name: "Configurations", Type: "GetJson"})
		var requestErr *recommendationRequestError
		if body != "" || !errors.As(err, &requestErr) || awsApplySafeErrorCode(err) != "configuration-request-failed" {
			t.Errorf("requestConfigurationRecommendations(%q) = %q, %v; want empty body and classified request error", response, body, err)
		}
	}
}

func TestCloudRecommendationRequestFailuresStopBeforeProviderOperations(t *testing.T) {
	logger := logging.Init("cloud-recommendation-errors", false, false, io.Discard)
	defer logger.Close()
	for _, body := range []string{"", `{"max_connections":"200"}`} {
		for _, provider := range []string{"aws", "gcp", "azure"} {
			t.Run(provider+"/"+body, func(t *testing.T) {
				r := &awsApplyRepeater{recommendations: body, err: errors.New("recommendation connection interrupted")}
				var code, status int
				var output string
				switch provider {
				case "aws":
					code, status, output = ApplyConfAwsRds(r, nil, *logger, awsApplyConfig("instance-custom", ""), AWSApplyAll, nil)
				case "gcp":
					code, status, output = ApplyConfGcpCloudSQL(r, nil, *logger, &config.Config{}, ApplyModeFull, nil)
				case "azure":
					code, status, output = ApplyConfAzureMySQL(r, nil, *logger, &config.Config{AzureSubscriptionID: "test", AzureResourceGroup: "test", AzureMySQLServer: "db"}, ApplyModeFull, true, nil)
				}
				if code == 0 || status != 4 || r.calls != 1 {
					t.Fatalf("%s request failure = %d/%d %q, requests=%d; want failure and one request", provider, code, status, output, r.calls)
				}
				want := "recommendation connection interrupted"
				if provider == "aws" {
					want = "configuration-request-failed" // AWS audit output must not expose arbitrary error text.
				}
				if !strings.Contains(output, want) || strings.Contains(output, "parse configuration recommendations JSON") {
					t.Fatalf("%s must report a request error, got %q", provider, output)
				}
			})
		}
	}
}

func TestAWSRecommendationTimeoutHasDistinctAuditReason(t *testing.T) {
	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	http.DefaultTransport = recommendationTimeoutTransport{}
	configuration := awsApplyConfig("instance-custom", "")
	logger := testAWSApplyLogger()
	r := repeater.NewReleemConfigurationsRepeater(configuration, logger)
	code, status, output := ApplyConfAwsRds(r, nil, logger, configuration, AWSApplyAll, nil)
	if code == 0 || status != 4 || !strings.Contains(output, "configuration-request-timeout") {
		t.Fatalf("ApplyConfAwsRds(timeout) = %d/%d %q", code, status, output)
	}
}

type recommendationTimeoutTransport struct{}

func (recommendationTimeoutTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, context.DeadlineExceeded
}

type selectedLocalApplyRepeater struct{ modes []models.ModeType }

func (r *selectedLocalApplyRepeater) ProcessMetrics(_ models.MetricContext, _ models.Metrics, mode models.ModeType) (string, error) {
	r.modes = append(r.modes, mode)
	if mode.Type == "GetJson" {
		return `{"max_connections":"200"}`, nil
	}
	return "[mysqld]\nmax_connections=200", nil
}

func TestLocalDynamicApplyPassesSelectionToBothSeparateRequests(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		logger := logging.Init("local-configuration-selection", false, false, io.Discard)
		defer logger.Close()
		metrics := &models.Metrics{}
		metrics.DB.Conf.Variables = models.MetricGroupValue{"max_connections": "200"}
		r := &selectedLocalApplyRepeater{}
		code, status, _ := applyDynamicConfigurationLocal(metrics, r, *logger, &config.Config{}, &models.Task{TypeID: 4, Details: `{"configuration_status_id":23}`}, func(taskCommand, logging.Logger) (int, int, string) { return 0, 1, "" })
		if code != 0 || status != 1 || len(r.modes) != 2 {
			t.Fatalf("applyDynamicConfigurationLocal = %d/%d, modes=%+v", code, status, r.modes)
		}
		for i, kind := range []string{"GetJson", "Get"} {
			if r.modes[i].Type != kind || r.modes[i].ConfigurationStatusID != 23 || r.modes[i].ApplyMode != "dynamic" {
				t.Fatalf("request %d = %+v, want %s with selection 23 and dynamic mode", i, r.modes[i], kind)
			}
		}
	})
}
