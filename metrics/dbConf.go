package metrics

import (
	"database/sql"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/advantageous/go-logback/logging"
)

type DbConfGatherer struct {
	logger logging.Logger
	debug  bool
	db     *sql.DB
}

func NewDbConfGatherer(logger logging.Logger, db *sql.DB, configuration *config.Config) *DbConfGatherer {

	if logger == nil {
		if configuration.Debug {
			logger = logging.NewSimpleDebugLogger("DbConf")
		} else {
			logger = logging.NewSimpleLogger("DbConf")
		}
	}

	return &DbConfGatherer{
		logger: logger,
		debug:  configuration.Debug,
		db:     db,
	}
}

func (DbConf *DbConfGatherer) GetMetrics(metrics *Metrics) error {

	output := make(MetricGroupValue)

	rows, err := DbConf.db.Query("SHOW VARIABLES")
	if err != nil {
		DbConf.logger.Error(err)
		DbConf.logger.Debug("collectMetrics ", output)
		return nil
	}
	defer rows.Close()

	for rows.Next() {
		var row MetricValue
		if err := rows.Scan(&row.name, &row.value); err != nil {
			DbConf.logger.Error(err)
		}
		output[row.name] = row.value
	}
	rows.Close()

	rows, err = DbConf.db.Query("SHOW GLOBAL VARIABLES")
	if err != nil {
		DbConf.logger.Error(err)
		DbConf.logger.Debug("collectMetrics ", output)
		return nil
	}
	defer rows.Close()

	for rows.Next() {
		var row MetricValue
		if err := rows.Scan(&row.name, &row.value); err != nil {
			DbConf.logger.Error(err)
		}
		output[row.name] = row.value
	}
	metrics.DB.Conf.Variables = output
	DbConf.logger.Debug("collectMetrics ", output)

	return nil

}
