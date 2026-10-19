package tasks

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	logging "github.com/google/logger"
)

func TestRecommendationRequestPreservesErrorsAndRejectsPartialBody(t *testing.T) {
	wantErr := errors.New("connection interrupted")
	r := &downloadCapture{body: `{"max_connections":"200"}`, err: wantErr}
	mode := models.ModeType{Name: "Configurations", Type: "GetJson", ApplyMode: "dynamic", ConfigurationStatusID: 23}
	body, err := requestConfigurationRecommendations(&models.Metrics{}, r, &config.Config{}, mode)
	if body != "" || !errors.Is(err, wantErr) || r.mode != mode {
		t.Fatalf("requestConfigurationRecommendations = %q, %v, mode %+v; want empty body, original error, mode %+v", body, err, r.mode, mode)
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
