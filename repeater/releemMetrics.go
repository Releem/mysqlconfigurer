package repeater

import (
	"io"
	"net/http"
	"strings"

	"encoding/json"

	"github.com/Releem/mysqlconfigurer/config"
	m "github.com/Releem/mysqlconfigurer/metrics"
	"github.com/advantageous/go-logback/logging"

	"time"
)

type ReleemMetricsRepeater struct {
	logger        logging.Logger
	configuration *config.Config
}

func (repeater ReleemMetricsRepeater) ProcessMetrics(context m.MetricContext, metrics m.Metrics) (interface{}, error) {
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
		Timeout: 30 * time.Second,
	}

	res, err := client.Do(req)
	if err != nil {
		repeater.logger.Error("Request: error making http request: ", err)
	}
	result_data := m.Task{}
	repeater.logger.Debug("Response: status code: ", res.StatusCode)
	if res.StatusCode == 200 {
		defer res.Body.Close()
		body_res, _ := io.ReadAll(res.Body)
		repeater.logger.Debug("Response: body:\n", string(body_res))
		err := json.Unmarshal(body_res, &result_data)
		if err != nil {
			repeater.logger.Error(err)
		}
	}
	return result_data, err
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
