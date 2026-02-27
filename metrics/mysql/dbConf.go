package mysql

import (
	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
	logging "github.com/google/logger"
)

type DbConfGatherer struct {
	logger        logging.Logger
	configuration *config.Config
}

func NewDBConfGatherer(logger logging.Logger, configuration *config.Config) *DbConfGatherer {
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
	DbConf.logger.V(5).Info("CollectMetrics DbConf ", output)

	return nil
}
