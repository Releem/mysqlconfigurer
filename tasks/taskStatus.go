package tasks

import (
	"net/http"
	"strings"

	"encoding/json"

	"github.com/Releem/mysqlconfigurer/config"
	m "github.com/Releem/mysqlconfigurer/metrics"
	"github.com/advantageous/go-logback/logging"

	"time"
)

type ReleemTaskStatusRepeater struct {
	logger logging.Logger
}

func (repeater ReleemTaskStatusRepeater) ProcessMetrics(context m.MetricContext, metrics m.Metrics) (interface{}, error) {
	e, _ := json.Marshal(metrics)
	bodyReader := strings.NewReader(string(e))
	repeater.logger.Debug("Result Send data: ", string(e))
	var api_domain string
	env := context.GetEnv()
	if env == "dev2" {
		api_domain = "https://api.dev2.releem.com/v1/tasks/task_status"
	} else if env == "dev" {
		api_domain = "https://api.dev.releem.com/v1/tasks/task_status"
	} else if env == "stage" {
		api_domain = "https://api.stage.releem.com/v1/tasks/task_status"
	} else {
		api_domain = "https://api.releem.com/v1/tasks/task_status"
	}
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
	if res.StatusCode != 201 {
		repeater.logger.Println("Response: status code: ", res.StatusCode)
		repeater.logger.Println("Response: \n", res)
	} else {
		repeater.logger.Debug("Response: status code: ", res.StatusCode)
	}
	return nil, err
}

func NewReleemTaskStatusRepeater(configuration *config.Config) ReleemTaskStatusRepeater {
	var logger logging.Logger
	if configuration.Debug {
		logger = logging.NewSimpleDebugLogger("ReleemRepeaterTaskStatus")
	} else {
		logger = logging.NewSimpleLogger("ReleemRepeaterTaskStatus")
	}
	return ReleemTaskStatusRepeater{logger}
}
