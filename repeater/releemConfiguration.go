package repeater

import (
	"io"
	"net/http"
	"os"
	"strings"

	"encoding/json"

	"github.com/Releem/mysqlconfigurer/config"
	m "github.com/Releem/mysqlconfigurer/metrics"
	"github.com/advantageous/go-logback/logging"

	"time"
)

type ReleemConfigurationsRepeater struct {
	logger        logging.Logger
	configuration *config.Config
}

func (repeater ReleemConfigurationsRepeater) ProcessMetrics(context m.MetricContext, metrics m.Metrics, Mode m.ModeT) (interface{}, error) {
	defer m.HandlePanic(repeater.configuration, repeater.logger)
	repeater.logger.Debug(Mode.Name, Mode.ModeType)
	e, _ := json.Marshal(metrics)
	bodyReader := strings.NewReader(string(e))
	repeater.logger.Debug("Result Send data: ", string(e))
	var api_domain, subdomain string
	env := context.GetEnv()

	if env == "dev2" {
		subdomain = "dev2."
	} else if env == "dev" {
		subdomain = "dev."
	} else if env == "stage" {
		subdomain = "stage."
	} else {
		subdomain = ""
	}

	if Mode.Name == "TaskSet" && Mode.ModeType == "queries_optimization" {
		api_domain = "https://api.queries." + subdomain + "releem.com/v2/"
	} else if Mode.Name == "Metrics" && Mode.ModeType == "QueryOptimization" {
		api_domain = "https://api.queries." + subdomain + "releem.com/v2/"
	} else {
		api_domain = "https://api." + subdomain + "releem.com/v2/"
	}

	if Mode.Name == "Configurations" {
		if Mode.ModeType == "set" {
			api_domain = api_domain + "mysql"
		} else if Mode.ModeType == "get" {
			api_domain = api_domain + "config"
		} else if Mode.ModeType == "get-json" {
			api_domain = api_domain + "config?json=1"
		} else {
			api_domain = api_domain + "mysql"
		}
	} else if Mode.Name == "Metrics" {
		api_domain = api_domain + "metrics"
	} else if Mode.Name == "Event" {
		api_domain = api_domain + "event/" + Mode.ModeType
	} else if Mode.Name == "TaskGet" {
		api_domain = api_domain + "task/task_get"
	} else if Mode.Name == "TaskSet" {
		api_domain = api_domain + "task/" + Mode.ModeType
	} else if Mode.Name == "TaskStatus" {
		api_domain = api_domain + "task/task_status"
	}
	repeater.logger.Debug(api_domain)

	req, err := http.NewRequest(http.MethodPost, api_domain, bodyReader)
	if err != nil {
		repeater.logger.Error("Request: could not create request: ", err)
		return nil, err
	}
	req.Header.Set("x-releem-api-key", context.GetApiKey())

	client := http.Client{
		Timeout: 180 * time.Second,
	}
	res, err := client.Do(req)
	if err != nil {
		repeater.logger.Error("Request: error making http request: ", err)
		return nil, err
	}
	defer res.Body.Close()

	body_res, err := io.ReadAll(res.Body)
	if err != nil {
		repeater.logger.Error("Response: error read body request: ", err)
		return nil, err
	}
	if res.StatusCode != 200 && res.StatusCode != 201 {
		repeater.logger.Error("Response: status code: ", res.StatusCode)
		repeater.logger.Error("Response: body:\n", string(body_res))
	} else {
		repeater.logger.Debug("Response: status code: ", res.StatusCode)
		repeater.logger.Debug("Response: body:\n", string(body_res))

		if Mode.Name == "Configurations" {
			err = os.WriteFile(context.GetReleemConfDir()+"/z_aiops_mysql.cnf", body_res, 0644)
			if err != nil {
				repeater.logger.Error("WriteFile: Error write to file: ", err)
				return nil, err
			}
			return string(body_res), err

		} else if Mode.Name == "Metrics" {
			return string(body_res), err
		} else if Mode.Name == "Event" {
			return nil, err
		} else if Mode.Name == "TaskGet" {
			result_data := m.Task{}
			err := json.Unmarshal(body_res, &result_data)
			return result_data, err
		} else if Mode.Name == "TaskSet" {
			return nil, err
		} else if Mode.Name == "TaskStatus" {
			return nil, err
		}
	}
	return nil, err
}

func NewReleemConfigurationsRepeater(configuration *config.Config) ReleemConfigurationsRepeater {
	var logger logging.Logger
	if configuration.Debug {
		logger = logging.NewSimpleDebugLogger("ReleemRepeaterConfigurations")
	} else {
		logger = logging.NewSimpleLogger("ReleemRepeaterConfigurations")
	}
	return ReleemConfigurationsRepeater{logger, configuration}
}
