package repeater

import (
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"encoding/json"

	"github.com/Releem/mysqlconfigurer/releem-agent/config"
	m "github.com/Releem/mysqlconfigurer/releem-agent/metrics"
	"github.com/advantageous/go-logback/logging"

	"time"
)

type ReleemConfigurationsRepeater struct {
	logger        logging.Logger
	configuration *config.Config
}

func (repeater ReleemConfigurationsRepeater) ProcessMetrics(context m.MetricContext, metrics m.Metrics) error {
	e, _ := json.Marshal(metrics)
	bodyReader := strings.NewReader(string(e))
	repeater.logger.Debug("Result Send data: ", string(e))
	var api_domain string
	if context.GetEnv() == "dev" {
		api_domain = "https://api.dev.releem.com/v2/mysql"
	} else {
		api_domain = "https://api.releem.com/v2/mysql"
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
	defer res.Body.Close()
	body_res, err := ioutil.ReadAll(res.Body)
	if err != nil {
		repeater.logger.Error("Response: error read body request: ", err)
	}
	err = os.WriteFile(context.GetReleemConfDir()+"/z_aiops_mysql.cnf", body_res, 0644)
	if err != nil {
		repeater.logger.Error("WriteFile: Error write to file: ", err)
	}
	repeater.logger.Debug("Response: status code: ", res.StatusCode)
	repeater.logger.Debug("Response: body:\n", string(body_res))

	return err
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
