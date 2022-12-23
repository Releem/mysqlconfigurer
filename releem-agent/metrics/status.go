package metrics

import (
	"database/sql"

	"github.com/Releem/mysqlconfigurer/releem-agent/config"
	"github.com/advantageous/go-logback/logging"
)

type MysqlStatusMetricsGatherer struct {
	logger logging.Logger
	debug  bool
	db     *sql.DB
}

func NewMysqlStatusMetricsGatherer(logger logging.Logger, db *sql.DB, configuration *config.Config) *MysqlStatusMetricsGatherer {

	if logger == nil {
		if configuration.Debug {
			logger = logging.NewSimpleDebugLogger("Status")
		} else {
			logger = logging.NewSimpleLogger("Status")
		}
	}

	return &MysqlStatusMetricsGatherer{
		logger: logger,
		debug:  configuration.Debug,
		db:     db,
	}
}

func (status *MysqlStatusMetricsGatherer) GetMetrics() (Metric, error) {

	output := make(MetricGroupValue)

	rows, err := status.db.Query("SHOW STATUS")
	if err != nil {
		status.logger.Error(err)
		return Metric{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var row MetricValue
		if err := rows.Scan(&row.name, &row.value); err != nil {
			status.logger.Error(err)
		}
		output[row.name] = row.value
	}
	rows.Close()

	rows, err = status.db.Query("SHOW GLOBAL STATUS")
	if err != nil {
		status.logger.Error(err)
		return Metric{}, err
	}
	for rows.Next() {
		var row MetricValue
		if err := rows.Scan(&row.name, &row.value); err != nil {
			status.logger.Error(err)
		}
		output[row.name] = row.value
	}
	metrics := Metric{"Status": output}
	status.logger.Debugf("collectMetrics %s", output)

	return metrics, nil

}
