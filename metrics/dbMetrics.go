package metrics

import (
	"time"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
	logging "github.com/google/logger"
)

type DbMetricsGatherer struct {
	logger        logging.Logger
	configuration *config.Config
}

func NewDbMetricsGatherer(logger logging.Logger, configuration *config.Config) *DbMetricsGatherer {
	return &DbMetricsGatherer{
		logger:        logger,
		configuration: configuration,
	}
}

func (DbMetrics *DbMetricsGatherer) GetMetrics(metrics *models.Metrics) error {
	defer utils.HandlePanic(DbMetrics.configuration, DbMetrics.logger)
	//list of databases
	{
		var database string
		var output []string
		rows, err := models.DB.Query("SELECT table_schema FROM INFORMATION_SCHEMA.tables group BY table_schema")
		if err != nil {
			DbMetrics.logger.Error(err)
			return err
		}
		for rows.Next() {
			err := rows.Scan(&database)
			if err != nil {
				DbMetrics.logger.Error(err)
				return err
			}
			output = append(output, database)
		}
		rows.Close()
		metrics.DB.Metrics.Databases = output
	}
	//Total table
	{
		var row uint64
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
		var size, count, dsize, isize uint64
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
			engine_elem[engine_db] = models.MetricGroupValue{"Table Number": uint64(0), "Total Size": uint64(0), "Data Size": uint64(0), "Index Size": uint64(0)}
		}
		rows.Close()
		i := 0
		for _, database := range metrics.DB.Metrics.Databases {
			rows, err = models.DB.Query(`SELECT ENGINE, IFNULL(SUM(DATA_LENGTH+INDEX_LENGTH), 0), IFNULL(COUNT(ENGINE), 0), IFNULL(SUM(DATA_LENGTH), 0), IFNULL(SUM(INDEX_LENGTH), 0) FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND ENGINE IS NOT NULL  GROUP BY ENGINE ORDER BY ENGINE ASC`, database)
			if err != nil {
				DbMetrics.logger.Error(err)
				return err
			}
			for rows.Next() {
				err := rows.Scan(&engine_db, &size, &count, &dsize, &isize)
				if err != nil {
					DbMetrics.logger.Error(err)
					continue
				}
				if engine_elem[engine_db]["Table Number"] == nil {
					engine_elem[engine_db] = models.MetricGroupValue{"Table Number": uint64(0), "Total Size": uint64(0), "Data Size": uint64(0), "Index Size": uint64(0)}
				}
				engine_elem[engine_db]["Table Number"] = engine_elem[engine_db]["Table Number"].(uint64) + count
				engine_elem[engine_db]["Total Size"] = engine_elem[engine_db]["Total Size"].(uint64) + size
				engine_elem[engine_db]["Data Size"] = engine_elem[engine_db]["Data Size"].(uint64) + dsize
				engine_elem[engine_db]["Index Size"] = engine_elem[engine_db]["Index Size"].(uint64) + isize
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
			metrics.DB.Metrics.TotalMyisamIndexes = metrics.DB.Metrics.Engine["MyISAM"]["Index Size"].(uint64)
		}
	}
	DbMetrics.logger.V(5).Info("CollectMetrics DbMetrics ", metrics.DB.Metrics)
	return nil

}
