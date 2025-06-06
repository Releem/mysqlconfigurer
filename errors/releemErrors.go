package errors

import (
	"net/http"
	"strings"

	"github.com/Releem/mysqlconfigurer/config"
	logging "github.com/google/logger"

	"time"
)

type ReleemErrorsRepeater struct {
	logger        logging.Logger
	configuration *config.Config
}

func (repeater ReleemErrorsRepeater) ProcessErrors(message string) interface{} {
	var env string
	bodyReader := strings.NewReader(message)

	repeater.logger.V(5).Info("Result Send data: ", message)
	var api_domain, domain string
	if repeater.configuration != nil {
		env = repeater.configuration.Env
	} else {
		env = "prod"
	}
	if repeater.configuration.ReleemRegion == "EU" {
		domain = "eu.releem.com"
	} else {
		domain = "releem.com"
	}
	if env == "dev2" {
		api_domain = "https://api.dev2." + domain + "/v2/events/agent_errors_log"
	} else if env == "dev" {
		api_domain = "https://api.dev." + domain + "/v2/events/agent_errors_log"
	} else if env == "stage" {
		api_domain = "https://api.stage." + domain + "/v2/events/agent_errors_log"
	} else {
		api_domain = "https://api." + domain + "/v2/events/agent_errors_log"
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
	repeater.logger.V(5).Info("Response: status code: ", res.StatusCode)
	return res
}

func NewReleemErrorsRepeater(configuration *config.Config, logger logging.Logger) ReleemErrorsRepeater {
	return ReleemErrorsRepeater{logger, configuration}
}
