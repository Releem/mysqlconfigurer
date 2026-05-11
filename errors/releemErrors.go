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
	var api_domain, domain, subdomain string
	if repeater.configuration != nil {
		env = repeater.configuration.Env
	} else {
		env = "prod"
	}
	switch env {
	case "dev2":
		subdomain = "dev2."
	case "dev":
		subdomain = "dev."
	case "stage":
		subdomain = "stage."
	default:
		subdomain = ""
	}
	if repeater.configuration.ReleemRegion == "EU" {
		domain = "eu.releem.com"
	} else {
		domain = "releem.com"
	}
	api_domain = "https://api.queries." + subdomain + domain + "/v2/events/agent_errors_log"

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
