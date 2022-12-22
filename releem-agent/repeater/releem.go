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

type ReleemMetricRepeater struct {
	logger        logging.Logger
	configuration *config.Config
}

func (repeater ReleemMetricRepeater) ProcessMetrics(context m.MetricContext, metrics m.Metric) error {
	e, _ := json.Marshal(metrics)
	bodyReader := strings.NewReader(string(e))
	repeater.logger.Debugf("Result Send data %s", e)
	var api_domain string
	if context.GetEnv() == "dev" {
		api_domain = "https://api.dev.releem.com/v1/metrics"
	} else {
		api_domain = "https://api.releem.com/v1/metrics"
	}
	req, err := http.NewRequest(http.MethodPost, api_domain, bodyReader)
	if err != nil {
		repeater.logger.Errorf("client: could not create request: %s\n", err)
	}
	req.Header.Set("x-releem-api-key", context.GetApiKey())

	client := http.Client{
		Timeout: 5 * time.Second,
	}

	res, err := client.Do(req)
	if err != nil {
		repeater.logger.Errorf("client: error making http request: %s\n", err)
	}
	repeater.logger.Debugf("client: status code: %s\n", res)
	return err
}

func NewReleemMetricsRepeater(configuration *config.Config) ReleemMetricRepeater {
	var logger logging.Logger
	if configuration.Debug {
		logger = logging.NewSimpleDebugLogger("ReleemRepeater")
	} else {
		logger = logging.NewSimpleLogger("ReleemRepeater")
	}
	return ReleemMetricRepeater{logger, configuration}
}
