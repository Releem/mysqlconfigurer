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

func (repeater ReleemConfigurationsRepeater) ProcessMetrics(context m.MetricContext, metrics m.Metrics) (interface{}, error) {
	repeater.logger.Println(" * Sending metrics to Releem Cloud Platform...")
	e, _ := json.Marshal(metrics)
	bodyReader := strings.NewReader(string(e))
	repeater.logger.Debug("Result Send data: ", string(e))
	var api_domain string
	env := context.GetEnv()
	if env == "dev" {
		api_domain = "https://api.dev.releem.com/v2/mysql"
	} else if env == "stage" {
		api_domain = "https://api.stage.releem.com/v2/mysql"
	} else {
		api_domain = "https://api.releem.com/v2/mysql"
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
	defer res.Body.Close()
	repeater.logger.Println(" * Downloading recommended MySQL configuration from Releem Cloud Platform...")

	body_res, err := io.ReadAll(res.Body)
	if err != nil {
		repeater.logger.Error("Response: error read body request: ", err)
		return nil, err
	}
	if res.StatusCode != 200 {
		repeater.logger.Println("Response: status code: ", res.StatusCode)
		repeater.logger.Println("Response: body:\n", string(body_res))
	} else {
		repeater.logger.Debug("Response: status code: ", res.StatusCode)
		repeater.logger.Debug("Response: body:\n", string(body_res))
		err = os.WriteFile(context.GetReleemConfDir()+"/z_aiops_mysql.cnf", body_res, 0644)
		if err != nil {
			repeater.logger.Error("WriteFile: Error write to file: ", err)
			return nil, err
		}
		repeater.logger.Println("1. Recommended MySQL configuration downloaded to ", context.GetReleemConfDir())
		repeater.logger.Println("2. To check MySQL Performance Score please visit https://app.releem.com/dashboard?menu=metrics")
		repeater.logger.Println("3. To apply the recommended configuration please read documentation https://app.releem.com/dashboard")
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
