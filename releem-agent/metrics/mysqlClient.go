package metrics

import (
	"database/sql"
	"regexp"

	"github.com/Releem/mysqlconfigurer/releem-agent/config"
	"github.com/advantageous/go-logback/logging"
)

type MysqlClientMetricsGatherer struct {
	logger logging.Logger
	debug  bool
	db     *sql.DB
}

func NewMysqlClientMetricsGatherer(logger logging.Logger, db *sql.DB, configuration *config.Config) *MysqlClientMetricsGatherer {

	if logger == nil {
		if configuration.Debug {
			logger = logging.NewSimpleDebugLogger("mysqlClient")
		} else {
			logger = logging.NewSimpleLogger("mysqlClient")
		}
	}

	return &MysqlClientMetricsGatherer{
		logger: logger,
		debug:  configuration.Debug,
		db:     db,
	}
}

func (mysqlClient *MysqlClientMetricsGatherer) GetMetrics() (Metric, error) {

	output := make(map[string]interface{})

	rows, err := mysqlClient.db.Query("select 'Version' as name, VERSION()")
	if err != nil {
		mysqlClient.logger.Error(err)
		metrics := Metric{"MySQL Client": output}
		mysqlClient.logger.Debug("collectMetrics ", output)
		return metrics, nil
	}
	defer rows.Close()

	for rows.Next() {
		var row MetricValue
		if err := rows.Scan(&row.name, &row.value); err != nil {
			mysqlClient.logger.Error(err)
		}
		re := regexp.MustCompile(`(.*?)\-.*`)
		output[row.name] = re.FindStringSubmatch(row.value)[1]
	}
	metrics := Metric{"MySQL Client": output}
	mysqlClient.logger.Debug("collectMetrics ", output)
	return metrics, nil

}
