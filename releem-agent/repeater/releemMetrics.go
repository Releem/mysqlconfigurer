package repeater

import (
	"net/http"
	"strings"

	"encoding/json"

	"github.com/Releem/mysqlconfigurer/releem-agent/config"
	m "github.com/Releem/mysqlconfigurer/releem-agent/metrics"
	"github.com/advantageous/go-logback/logging"

	"time"
)

type ReleemMetricsRepeater struct {
	logger        logging.Logger
	configuration *config.Config
}

func (repeater ReleemMetricsRepeater) ProcessMetrics(context m.MetricContext, metrics m.Metrics) error {
	e, _ := json.Marshal(metrics)
	bodyReader := strings.NewReader(string(e))
	repeater.logger.Debug("Result Send data: ", string(e))
	var api_domain string
	env := context.GetEnv()
	if env == "dev" {
		api_domain = "https://api.dev.releem.com/v2/metrics"
	} else if env == "stage" {
		api_domain = "https://api.stage.releem.com/v2/metrics"
	} else {
		api_domain = "https://api.releem.com/v2/metrics"
	}
	req, err := http.NewRequest(http.MethodPost, api_domain, bodyReader)
	if err != nil {
		repeater.logger.Error("Request: could not create request: ", err)
	}
	req.Header.Set("x-releem-api-key", context.GetApiKey())

	client := http.Client{
		Timeout: 5 * time.Second,
	}

	res, err := client.Do(req)
	if err != nil {
		repeater.logger.Error("Request: error making http request: ", err)
	}
	repeater.logger.Debug("Response: status code: ", res)
	return err
}

func NewReleemMetricsRepeater(configuration *config.Config) ReleemMetricsRepeater {
	var logger logging.Logger
	if configuration.Debug {
		logger = logging.NewSimpleDebugLogger("ReleemRepeaterMetrics")
	} else {
		logger = logging.NewSimpleLogger("ReleemRepeaterMetrics")
	}
	return ReleemMetricsRepeater{logger, configuration}
}
