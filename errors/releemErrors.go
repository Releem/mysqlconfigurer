package errors

import (
	"net/http"
	"strings"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/advantageous/go-logback/logging"

	"time"
)

type ReleemErrorsRepeater struct {
	logger        logging.Logger
	configuration *config.Config
}

func (repeater ReleemErrorsRepeater) ProcessErrors(message string) interface{} {
	var env string
	bodyReader := strings.NewReader(message)

	repeater.logger.Debug("Result Send data: ", message)
	var api_domain string
	if repeater.configuration != nil {
		env = repeater.configuration.Env
	} else {
		env = "prod"
	}
	if env == "dev2" {
		api_domain = "https://api.dev2.releem.com/v2/events/agent_errors_log"
	} else if env == "dev" {
		api_domain = "https://api.dev.releem.com/v2/events/agent_errors_log"
	} else if env == "stage" {
		api_domain = "https://api.stage.releem.com/v2/events/agent_errors_log"
	} else {
		api_domain = "https://api.releem.com/v2/events/agent_errors_log"
	}
	req, err := http.NewRequest(http.MethodPost, api_domain, bodyReader)
	if err != nil {
		repeater.logger.Error("Request: could not create request: ", err)
		return nil
	}
	if repeater.configuration != nil {
		req.Header.Set("x-releem-api-key", repeater.configuration.ApiKey)
	}

	client := http.Client{
		Timeout: 30 * time.Second,
	}

	res, err := client.Do(req)
	if err != nil {
		repeater.logger.Error("Request: error making http request: ", err)
		return nil
	}
	repeater.logger.Debug("Response: status code: ", res.StatusCode)
	return nil
}

func NewReleemErrorsRepeater(configuration *config.Config) ReleemErrorsRepeater {
	logger := logging.NewSimpleLogger("ReleemRepeaterMetrics")
	return ReleemErrorsRepeater{logger, configuration}
}
