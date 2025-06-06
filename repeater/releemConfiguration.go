package repeater

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
	logging "github.com/google/logger"

	"time"
)

type ReleemConfigurationsRepeater struct {
	logger        logging.Logger
	configuration *config.Config
}

func (repeater ReleemConfigurationsRepeater) ProcessMetrics(context models.MetricContext, metrics models.Metrics, Mode models.ModeType) (interface{}, error) {
	defer utils.HandlePanic(repeater.configuration, repeater.logger)
	repeater.logger.V(5).Info(Mode.Name, Mode.Type)
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	if err := encoder.Encode(metrics); err != nil {
		repeater.logger.Error("Failed to encode metrics: ", err)
	}
	repeater.logger.V(5).Info("Result Send data: ", buffer.String())
	var api_domain, subdomain, domain string
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
	if repeater.configuration.ReleemRegion == "EU" {
		domain = "eu.releem.com"
	} else {
		domain = "releem.com"
	}
	if Mode.Name == "TaskSet" && Mode.Type == "queries_optimization" {
		api_domain = "https://api.queries." + subdomain + domain + "/v2/"
	} else if Mode.Name == "Metrics" {
		api_domain = "https://api.queries." + subdomain + domain + "/v2/"
	} else {
		api_domain = "https://api." + subdomain + domain + "/v2/"
	}

	if Mode.Name == "Configurations" {
		if Mode.Type == "set" {
			api_domain = api_domain + "mysql"
		} else if Mode.Type == "get" {
			api_domain = api_domain + "config"
		} else if Mode.Type == "get-json" {
			api_domain = api_domain + "config?json=1"
		} else {
			api_domain = api_domain + "mysql"
		}
	} else if Mode.Name == "Metrics" {
		if Mode.Type == "QueryOptimization" {
			api_domain = api_domain + "queries/metrics"
		} else {
			api_domain = api_domain + "mysql/metrics"
		}
	} else if Mode.Name == "Event" {
		api_domain = api_domain + "event/" + Mode.Type
	} else if Mode.Name == "TaskGet" {
		api_domain = api_domain + "task/task_get"
	} else if Mode.Name == "TaskSet" {
		api_domain = api_domain + "task/" + Mode.Type
	} else if Mode.Name == "TaskStatus" {
		api_domain = api_domain + "task/task_status"
	}
	repeater.logger.V(5).Info(api_domain)

	req, err := http.NewRequest(http.MethodPost, api_domain, &buffer)
	if err != nil {
		repeater.logger.Error("Request: could not create request: ", err)
		return nil, err
	}
	req.Header.Set("x-releem-api-key", context.GetApiKey())

	client := http.Client{
		Timeout: 10 * time.Minute,
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
		repeater.logger.V(5).Info("Response: status code: ", res.StatusCode)
		repeater.logger.V(5).Info("Response: body:\n", string(body_res))

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
			result_data := models.Task{}
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

func NewReleemConfigurationsRepeater(configuration *config.Config, logger logging.Logger) ReleemConfigurationsRepeater {
	return ReleemConfigurationsRepeater{logger, configuration}
}
