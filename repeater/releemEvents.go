package repeater

import (
	"net/http"
	"strings"

	"encoding/json"

	"github.com/Releem/mysqlconfigurer/config"
	m "github.com/Releem/mysqlconfigurer/metrics"
	"github.com/advantageous/go-logback/logging"

	"time"
)

type ReleemEventRepeater struct {
	logger        logging.Logger
	configuration *config.Config
	Mode          m.Mode
}

func (repeater ReleemEventRepeater) ProcessMetrics(context m.MetricContext, metrics m.Metrics) (interface{}, error) {
	e, _ := json.Marshal(metrics)
	bodyReader := strings.NewReader(string(e))
	repeater.logger.Debug("Result Send data: ", string(e))
	var api_domain string
	env := context.GetEnv()
	if env == "dev2" {
		api_domain = "https://api.dev2.releem.com/v2/event/"
	} else if env == "dev" {
		api_domain = "https://api.dev.releem.com/v2/event/"
	} else if env == "stage" {
		api_domain = "https://api.stage.releem.com/v2/event/"
	} else {
		api_domain = "https://api.releem.com/v2/event/"
	}
	api_domain += repeater.Mode.ModeType

	req, err := http.NewRequest(http.MethodPost, api_domain, bodyReader)
	if err != nil {
		repeater.logger.Error("Request: could not create request: ", err)
		return nil, err
	}
	req.Header.Set("x-releem-api-key", context.GetApiKey())

	client := http.Client{
		Timeout: 30 * time.Second,
	}

	res, err := client.Do(req)
	if err != nil {
		repeater.logger.Error("Request: error making http request: ", err)
		return nil, err
	}
	repeater.logger.Debug("Response: status code: ", res)
	return nil, err
}

func NewReleemEventRepeater(configuration *config.Config, Mode m.Mode) ReleemEventRepeater {
	var logger logging.Logger
	if configuration.Debug {
		logger = logging.NewSimpleDebugLogger("ReleemRepeaterMetrics")
	} else {
		logger = logging.NewSimpleLogger("ReleemRepeaterMetrics")
	}
	return ReleemEventRepeater{logger, configuration, Mode}
}
