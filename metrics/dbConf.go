package metrics

import (
	"github.com/Releem/mysqlconfigurer/config"
	"github.com/advantageous/go-logback/logging"
)

type DbConfGatherer struct {
	logger        logging.Logger
	configuration *config.Config
}

func NewDbConfGatherer(logger logging.Logger, configuration *config.Config) *DbConfGatherer {

	if logger == nil {
		if configuration.Debug {
			logger = logging.NewSimpleDebugLogger("DbConf")
		} else {
			logger = logging.NewSimpleLogger("DbConf")
		}
	}

	return &DbConfGatherer{
		logger:        logger,
		configuration: configuration,
	}
}

func (DbConf *DbConfGatherer) GetMetrics(metrics *Metrics) error {
	defer HandlePanic(DbConf.configuration, DbConf.logger)

	output := make(MetricGroupValue)

	rows, err := config.DB.Query("SHOW VARIABLES")
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

	rows, err = config.DB.Query("SHOW GLOBAL VARIABLES")
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
