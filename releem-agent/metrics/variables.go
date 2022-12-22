package metrics

import (
	"database/sql"

	"github.com/Releem/mysqlconfigurer/releem-agent/config"
	"github.com/advantageous/go-logback/logging"
)

type MysqlVariablesMetricsGatherer struct {
	logger logging.Logger
	debug  bool
	db     *sql.DB
}

func NewMysqlVariablesMetricsGatherer(logger logging.Logger, db *sql.DB, configuration *config.Config) *MysqlVariablesMetricsGatherer {

	if logger == nil {
		if configuration.Debug {
			logger = logging.NewSimpleDebugLogger("Variables")
		} else {
			logger = logging.NewSimpleLogger("Variables")
		}
	}

	return &MysqlVariablesMetricsGatherer{
		logger: logger,
		debug:  configuration.Debug,
		db:     db,
	}
}

func (variables *MysqlVariablesMetricsGatherer) GetMetrics() (Metric, error) {

	output := make(MetricGroupValue)

	rows, err := variables.db.Query("SHOW VARIABLES")
	if err != nil {
		variables.logger.Error(err)
		return Metric{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var row MetricValue
		if err := rows.Scan(&row.name, &row.value); err != nil {
			variables.logger.Error(err)
		}
		output[row.name] = row.value
	}
	rows.Close()

	rows, err = variables.db.Query("SHOW GLOBAL VARIABLES")
	if err != nil {
		variables.logger.Error(err)
		return Metric{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var row MetricValue
		if err := rows.Scan(&row.name, &row.value); err != nil {
			variables.logger.Error(err)
		}
		output[row.name] = row.value
	}
	metrics := Metric{"Variables": output}
	variables.logger.Debugf("collectMetrics %s", output)

	return metrics, nil

}
