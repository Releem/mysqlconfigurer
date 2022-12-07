package metrics

import (
	"database/sql"

	"github.com/Releem/mysqlconfigurer/releem-agent/config"
	"github.com/advantageous/go-logback/logging"
)

type MysqlCalculationsMetricsGatherer struct {
	logger logging.Logger
	debug  bool
	db     *sql.DB
}

func NewMysqlCalculationsMetricsGatherer(logger logging.Logger, db *sql.DB, configuration *config.Config) *MysqlCalculationsMetricsGatherer {

	if logger == nil {
		if configuration.Debug {
			logger = logging.NewSimpleDebugLogger("Calculations")
		} else {
			logger = logging.NewSimpleLogger("Calculations")
		}
	}

	return &MysqlCalculationsMetricsGatherer{
		logger: logger,
		debug:  configuration.Debug,
		db:     db,
	}
}

func (calculations *MysqlCalculationsMetricsGatherer) GetMetrics() (Metric, error) {

	output := make(map[string]interface{})

	rows, err := calculations.db.Query("SELECT 'total_tables' as name, COUNT(*) as count FROM information_schema.tables")
	if err != nil {
		calculations.logger.Error(err)
		metrics := Metric{"AgentCalculations": output}
		calculations.logger.Debug("collectMetrics ", output)
		return metrics, nil
	}
	defer rows.Close()

	for rows.Next() {
		var row MetricValue
		if err := rows.Scan(&row.name, &row.value); err != nil {
			calculations.logger.Error(err)
		}
		output[row.name] = row.value
	}
	rows.Close()

	rows, err = calculations.db.Query("SELECT 'total_myisam_indexes' as name, IFNULL(SUM(INDEX_LENGTH), 0) FROM information_schema.TABLES WHERE TABLE_SCHEMA NOT IN ('information_schema') AND ENGINE = 'MyISAM'")
	if err != nil {
		calculations.logger.Error(err)
		metrics := Metric{"AgentCalculations": output}
		calculations.logger.Debug("collectMetrics ", output)
		return metrics, nil
	}

	for rows.Next() {
		var row MetricValue
		if err := rows.Scan(&row.name, &row.value); err != nil {
			calculations.logger.Error(err)
		}
		output[row.name] = row.value
	}

	metrics := Metric{"AgentCalculations": output}
	calculations.logger.Debug("collectMetrics ", output)

	return metrics, nil

}
