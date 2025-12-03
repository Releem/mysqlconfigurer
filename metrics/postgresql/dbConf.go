package postgresql

import (
	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
	logging "github.com/google/logger"
)

type DBConfGatherer struct {
	logger        logging.Logger
	configuration *config.Config
}

func NewDBConfGatherer(logger logging.Logger, configuration *config.Config) *DBConfGatherer {
	return &DBConfGatherer{
		logger:        logger,
		configuration: configuration,
	}
}

func (DBConf *DBConfGatherer) GetMetrics(metrics *models.Metrics) error {
	defer utils.HandlePanic(DBConf.configuration, DBConf.logger)

	output := make(models.MetricGroupValue)

	// Get PostgreSQL settings from pg_settings
	rows, err := models.DB.Query(`
		SELECT name, setting, COALESCE(unit, '') as unit, category, short_desc, context, vartype, source, 
		       COALESCE(min_val, '') as min_val, COALESCE(max_val, '') as max_val, 
		       COALESCE(enumvals::text, '') as enumvals, COALESCE(boot_val, '') as boot_val, 
		       COALESCE(reset_val, '') as reset_val, pending_restart
		FROM pg_settings 
		ORDER BY name`)
	if err != nil {
		DBConf.logger.Error(err)
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var name, setting, unit, category, short_desc, context, vartype, source, min_val, max_val, enumvals, boot_val, reset_val string
		var pending_restart bool

		if err := rows.Scan(&name, &setting, &unit, &category, &short_desc, &context, &vartype, &source, &min_val, &max_val, &enumvals, &boot_val, &reset_val, &pending_restart); err != nil {
			DBConf.logger.Error(err)
			continue
		}

		// Store the setting value (similar to MySQL SHOW VARIABLES)
		output[name] = models.MetricGroupValue{
			"setting":         setting,
			"unit":            unit,
			"category":        category,
			"short_desc":      short_desc,
			"context":         context,
			"vartype":         vartype,
			"source":          source,
			"min_val":         min_val,
			"max_val":         max_val,
			"enumvals":        enumvals,
			"boot_val":        boot_val,
			"reset_val":       reset_val,
			"pending_restart": pending_restart,
		}
	}

	metrics.DB.Conf.Variables = output
	DBConf.logger.V(5).Info("CollectMetrics DBConf ", len(output), " settings collected")

	return nil
}
