package metrics

import (
	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
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

func (DbConf *DbConfGatherer) GetMetrics(metrics *models.Metrics) error {
	defer utils.HandlePanic(DbConf.configuration, DbConf.logger)

	output := make(models.MetricGroupValue)

	rows, err := models.DB.Query("SHOW VARIABLES")
	if err != nil {
		DbConf.logger.Error(err)
		DbConf.logger.Debug("collectMetrics ", output)
		return nil
	}
	defer rows.Close()

	for rows.Next() {
		var row models.MetricValue

		if err := rows.Scan(&row.Name, &row.Value); err != nil {
			DbConf.logger.Error(err)
		}
		output[row.Name] = row.Value
	}
	rows.Close()

	rows, err = models.DB.Query("SHOW GLOBAL VARIABLES")
	if err != nil {
		DbConf.logger.Error(err)
		DbConf.logger.Debug("collectMetrics ", output)
		return nil
	}
	defer rows.Close()

	for rows.Next() {
		var row models.MetricValue
		if err := rows.Scan(&row.Name, &row.Value); err != nil {
			DbConf.logger.Error(err)
		}
		output[row.Name] = row.Value
	}
	metrics.DB.Conf.Variables = output
	DbConf.logger.Debug("collectMetrics ", output)

	return nil

}
