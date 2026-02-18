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
		SELECT name, 
			case when source = 'session' then reset_val else setting end as setting, 
			COALESCE(unit, 'NULL') as unit, 
			COALESCE(vartype, 'NULL') as vartype, 
			COALESCE(source, 'NULL') as source, 
			COALESCE(sourcefile, 'NULL') as sourcefile, 
			COALESCE(sourceline::text, 'NULL') as sourceline, 
			COALESCE(min_val, 'NULL') as min_val, 
			COALESCE(max_val, 'NULL') as max_val, 
			COALESCE(enumvals::text, 'NULL') as enumvals, 
			pending_restart
		FROM pg_settings`)
	if err != nil {
		DBConf.logger.Error(err)
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var name, setting, unit, vartype, source, sourcefile, sourceline, min_val, max_val, enumvals string
		var pending_restart bool

		if err := rows.Scan(&name, &setting, &unit, &vartype, &source, &sourcefile, &sourceline, &min_val, &max_val, &enumvals, &pending_restart); err != nil {
			DBConf.logger.Error(err)
			continue
		}

		// Store the setting value (similar to MySQL SHOW VARIABLES)
		output[name] = models.MetricGroupValue{
			"setting":         setting,
			"unit":            unit,
			"vartype":         vartype,
			"source":          source,
			"sourcefile":      sourcefile,
			"sourceline":      sourceline,
			"min_val":         min_val,
			"max_val":         max_val,
			"enumvals":        enumvals,
			"pending_restart": pending_restart,
		}
	}

	metrics.DB.Conf.Variables = output
	DBConf.logger.V(5).Info("CollectMetrics DBConf ", len(output), " settings collected")

	return nil
}
