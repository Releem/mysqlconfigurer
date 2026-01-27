package mysql

import (
	"maps"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
	logging "github.com/google/logger"
)

const (
	CollectSampleQueriesCurrent = "SELECT t2.`CURRENT_SCHEMA`, t2.`DIGEST`, t2.`SQL_TEXT` FROM (SELECT `CURRENT_SCHEMA`, `DIGEST`, MAX(`TIMER_START`) AS MAX_TIMER_START FROM `performance_schema`.`events_statements_current` WHERE `DIGEST` IS NOT NULL AND `CURRENT_SCHEMA` IS NOT NULL GROUP BY `CURRENT_SCHEMA`, `DIGEST` ) t1 JOIN `performance_schema`.`events_statements_current` t2 ON t2.`TIMER_START`=t1.`MAX_TIMER_START` AND t2.`CURRENT_SCHEMA`=t1.`CURRENT_SCHEMA` AND t2.`DIGEST`=t1.`DIGEST`"
	CollectSampleQueriesHistory = "SELECT t2.`CURRENT_SCHEMA`, t2.`DIGEST`, t2.`SQL_TEXT` FROM (SELECT `CURRENT_SCHEMA`, `DIGEST`, MAX(`TIMER_START`) AS MAX_TIMER_START FROM `performance_schema`.`events_statements_history` WHERE `DIGEST` IS NOT NULL AND `CURRENT_SCHEMA` IS NOT NULL GROUP BY `CURRENT_SCHEMA`, `DIGEST` ) t1 JOIN `performance_schema`.`events_statements_history` t2 ON t2.`TIMER_START`=t1.`MAX_TIMER_START` AND t2.`CURRENT_SCHEMA`=t1.`CURRENT_SCHEMA` AND t2.`DIGEST`=t1.`DIGEST`"
)

type DBCollectSampleQueriesGatherer struct {
	logger        logging.Logger
	configuration *config.Config
}

func NewDBCollectSampleQueriesGatherer(logger logging.Logger, configuration *config.Config) *DBCollectSampleQueriesGatherer {
	return &DBCollectSampleQueriesGatherer{logger: logger, configuration: configuration}
}

func (DBCollectSampleQueries *DBCollectSampleQueriesGatherer) GetMetrics(metrics *models.Metrics) error {
	defer utils.HandlePanic(DBCollectSampleQueries.configuration, DBCollectSampleQueries.logger)
	// Only collect SQL text for MySQL
	CollectSampleQueries := CollectSampleQueriesCurrent

	if DBCollectSampleQueries.configuration.CollectSampleQueriesPeriod == 1 {
		CollectSampleQueries = CollectSampleQueriesCurrent
	} else if DBCollectSampleQueries.configuration.CollectSampleQueriesPeriod == 10 {
		CollectSampleQueries = CollectSampleQueriesHistory
	} else {
		DBCollectSampleQueries.logger.Info("* SQL text collection is disabled...")
		return nil
	}
	rows, err := models.DB.Query(CollectSampleQueries)
	if err != nil {
		DBCollectSampleQueries.logger.Error(err)
		return err
	} else {
		defer rows.Close()

		// Batch collect data to reduce mutex contention
		tempData := make(map[string]string)
		var schema_name, query_id, sqlText string

		for rows.Next() {
			err := rows.Scan(&schema_name, &query_id, &sqlText)
			if err != nil {
				DBCollectSampleQueries.logger.Error(err)
				return err
			}
			tempData[schema_name+query_id] = sqlText
		}

		// Single mutex lock for batch update
		if len(tempData) > 0 {
			models.SampleQueriesMutex.Lock()
			maps.Copy(models.SampleQueries, tempData)
			models.SampleQueriesMutex.Unlock()
		}
	}
	return nil
}
