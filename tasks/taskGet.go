package tasks

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

type ReleemTasksRepeater struct {
	logger logging.Logger
}

func (repeater ReleemTasksRepeater) ProcessMetrics(context m.MetricContext, metrics m.Metrics) (interface{}, error) {
	result_data := m.Task{}
	e, _ := json.Marshal(metrics)
	bodyReader := strings.NewReader(string(e))
	repeater.logger.Debug("Result Send data: ", string(e))
	var api_domain string
	env := context.GetEnv()
	if env == "dev2" {
		api_domain = "https://api.dev2.releem.com/v2/tasks/task_get"
	} else if env == "dev" {
		api_domain = "https://api.dev.releem.com/v2/tasks/task_get"
	} else if env == "stage" {
		api_domain = "https://api.stage.releem.com/v2/tasks/task_get"
	} else {
		api_domain = "https://api.releem.com/v2/tasks/task_get"
	}
	req, err := http.NewRequest(http.MethodPost, api_domain, bodyReader)
	if err != nil {
		repeater.logger.Error("Request: could not create request: ", err)
		return result_data, err
	}
	req.Header.Set("x-releem-api-key", context.GetApiKey())
	client := http.Client{
		Timeout: 30 * time.Second,
	}
	res, err := client.Do(req)
	if err != nil {
		repeater.logger.Error("Request: error making http request: ", err)
		return result_data, err
	}
	defer res.Body.Close()

	body_res, err := io.ReadAll(res.Body)
	if err != nil {
		repeater.logger.Error("Response: error read body request: ", err)
		return result_data, err
	}
	if res.StatusCode != 200 {
		repeater.logger.Println("Response: status code: ", res.StatusCode)
		repeater.logger.Println("Response: body:\n", string(body_res))
	} else {
		repeater.logger.Debug("Response: status code: ", res.StatusCode)
		repeater.logger.Debug("Response: body:\n", string(body_res))
		err := json.Unmarshal(body_res, &result_data)

		if err != nil {
			repeater.logger.Error(err)
		}
	}
	return result_data, err

}

func NewReleemTasksRepeater(configuration *config.Config) ReleemTasksRepeater {
	var logger logging.Logger
	if configuration.Debug {
		logger = logging.NewSimpleDebugLogger("ReleemRepeaterTasks")
	} else {
		logger = logging.NewSimpleLogger("ReleemRepeaterTasks")
	}
	return ReleemTasksRepeater{logger}
}
