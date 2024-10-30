package metrics

import (
	"time"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
	"github.com/advantageous/go-logback/logging"
)

type DbMetricsGatherer struct {
	logger        logging.Logger
	configuration *config.Config
}

func NewDbMetricsGatherer(logger logging.Logger, configuration *config.Config) *DbMetricsGatherer {

	if logger == nil {
		if configuration.Debug {
			logger = logging.NewSimpleDebugLogger("DbMetrics")
		} else {
			logger = logging.NewSimpleLogger("DbMetrics")
		}
	}

	return &DbMetricsGatherer{
		logger:        logger,
		configuration: configuration,
	}
}

func (DbMetrics *DbMetricsGatherer) GetMetrics(metrics *models.Metrics) error {
	defer utils.HandlePanic(DbMetrics.configuration, DbMetrics.logger)
	//Total table
	{
		var row int
		err := models.DB.QueryRow("SELECT COUNT(*) as count FROM information_schema.tables").Scan(&row)
		if err != nil {
			DbMetrics.logger.Error(err)
			return err
		}
		metrics.DB.Metrics.TotalTables = row
	}
	//Stat mysql Engine
	{
		var engine_db, engineenabled string
		var size, count, dsize, isize int
		output := make(map[string]models.MetricGroupValue)
		engine_elem := make(map[string]models.MetricGroupValue)

		rows, err := models.DB.Query("SELECT ENGINE,SUPPORT FROM information_schema.ENGINES ORDER BY ENGINE ASC")
		if err != nil {
			DbMetrics.logger.Error(err)
			return err
		}
		for rows.Next() {
			err := rows.Scan(&engine_db, &engineenabled)
			if err != nil {
				DbMetrics.logger.Error(err)
				return err
			}
			output[engine_db] = models.MetricGroupValue{"Enabled": engineenabled}
			engine_elem[engine_db] = models.MetricGroupValue{"Table Number": 0, "Total Size": 0, "Data Size": 0, "Index Size": 0}
		}
		rows.Close()
		i := 0
		for _, database := range metrics.DB.Metrics.Databases {
			if database == "information_schema" || database == "performance_schema" || database == "mysql" {
				continue
			}
			rows, err = models.DB.Query(`SELECT ENGINE, IFNULL(SUM(DATA_LENGTH+INDEX_LENGTH), 0), IFNULL(COUNT(ENGINE), 0), IFNULL(SUM(DATA_LENGTH), 0), IFNULL(SUM(INDEX_LENGTH), 0) FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND ENGINE IS NOT NULL  GROUP BY ENGINE ORDER BY ENGINE ASC`, database)
			if err != nil {
				DbMetrics.logger.Error(err)
				return err
			}
			for rows.Next() {
				err := rows.Scan(&engine_db, &size, &count, &dsize, &isize)
				if err != nil {
					DbMetrics.logger.Error(err)
					return err
				}
				if engine_elem[engine_db]["Table Number"] == nil {
					engine_elem[engine_db] = models.MetricGroupValue{"Table Number": 0, "Total Size": 0, "Data Size": 0, "Index Size": 0}
				}
				engine_elem[engine_db]["Table Number"] = engine_elem[engine_db]["Table Number"].(int) + count
				engine_elem[engine_db]["Total Size"] = engine_elem[engine_db]["Total Size"].(int) + size
				engine_elem[engine_db]["Data Size"] = engine_elem[engine_db]["Data Size"].(int) + dsize
				engine_elem[engine_db]["Index Size"] = engine_elem[engine_db]["Index Size"].(int) + isize
			}
			rows.Close()
			i += 1
			if i%25 == 0 {
				time.Sleep(3 * time.Second)
			}
		}
		for k := range output {
			output[k] = utils.MapJoin(output[k], engine_elem[k])
		}

		metrics.DB.Metrics.Engine = output
		if metrics.DB.Metrics.Engine["MyISAM"] == nil {
			metrics.DB.Metrics.TotalMyisamIndexes = 0
		} else {
			metrics.DB.Metrics.TotalMyisamIndexes = metrics.DB.Metrics.Engine["MyISAM"]["Index Size"].(int)
		}
	}

	DbMetrics.logger.Debug("collectMetrics ", metrics.DB.Metrics)
	return nil

}
