package metrics

import (
	"github.com/Releem/mysqlconfigurer/config"
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

func (DbMetrics *DbMetricsGatherer) GetMetrics(metrics *Metrics) error {
	defer HandlePanic(DbMetrics.configuration, DbMetrics.logger)
	//Total table
	{
		var row MetricValue
		err := config.DB.QueryRow("SELECT COUNT(*) as count FROM information_schema.tables").Scan(&row.value)
		if err != nil {
			DbMetrics.logger.Error(err)
			return err
		}
		metrics.DB.Metrics.TotalTables = row.value
	}
	//list of databases
	{
		var database string
		var output []string
		rows, err := config.DB.Query("SELECT table_schema FROM INFORMATION_SCHEMA.tables group BY table_schema")
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
	// TotalMyisamIndexes
	{
		var row MetricValue
		err := config.DB.QueryRow("SELECT IFNULL(SUM(INDEX_LENGTH), 0) FROM information_schema.TABLES WHERE TABLE_SCHEMA NOT IN ('information_schema') AND ENGINE = 'MyISAM'").Scan(&row.value)
		if err != nil {
			DbMetrics.logger.Error(err)
			return err
		}
		metrics.DB.Metrics.TotalMyisamIndexes = row.value
	}
	//Stat mysql Engine
	{
		output := make(map[string]MetricGroupValue)
		var engine_db, size, count, dsize, isize, engineenabled string

		rows, err := config.DB.Query("SELECT ENGINE,SUPPORT FROM information_schema.ENGINES ORDER BY ENGINE ASC")
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
			output[engine_db] = MetricGroupValue{"Enabled": engineenabled}
			//output[engine_db]["Enabled"] = engineenabled
		}
		rows.Close()

		rows, err = config.DB.Query("SELECT ENGINE, IFNULL(SUM(DATA_LENGTH+INDEX_LENGTH),0), IFNULL(COUNT(ENGINE),0), IFNULL(SUM(DATA_LENGTH),0), IFNULL(SUM(INDEX_LENGTH),0) FROM information_schema.TABLES WHERE TABLE_SCHEMA NOT IN ('information_schema', 'performance_schema', 'mysql') AND ENGINE IS NOT NULL  GROUP BY ENGINE ORDER BY ENGINE ASC")
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
			engine_elem := MetricGroupValue{"Table Number": count, "Total Size": size, "Data Size": dsize, "Index Size": isize}
			if _, ok := output[engine_db]; ok {
				output[engine_db] = MapJoin(output[engine_db], engine_elem)
			}
		}
		metrics.DB.Metrics.Engine = output
		rows.Close()
	}

	DbMetrics.logger.Debug("collectMetrics ", metrics.DB.Metrics)
	return nil

}
