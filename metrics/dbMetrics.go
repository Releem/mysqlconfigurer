package metrics

import (
	"database/sql"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/advantageous/go-logback/logging"
)

type DbMetricsGatherer struct {
	logger logging.Logger
	debug  bool
	db     *sql.DB
}

func NewDbMetricsGatherer(logger logging.Logger, db *sql.DB, configuration *config.Config) *DbMetricsGatherer {

	if logger == nil {
		if configuration.Debug {
			logger = logging.NewSimpleDebugLogger("DbMetrics")
		} else {
			logger = logging.NewSimpleLogger("DbMetrics")
		}
	}

	return &DbMetricsGatherer{
		logger: logger,
		debug:  configuration.Debug,
		db:     db,
	}
}

func (DbMetrics *DbMetricsGatherer) GetMetrics(metrics *Metrics) error {
	//Total table
	{
		var row MetricValue
		err := DbMetrics.db.QueryRow("SELECT COUNT(*) as count FROM information_schema.tables").Scan(&row.value)
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
		rows, err := DbMetrics.db.Query("SELECT table_schema FROM INFORMATION_SCHEMA.tables group BY table_schema")
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
		err := DbMetrics.db.QueryRow("SELECT IFNULL(SUM(INDEX_LENGTH), 0) FROM information_schema.TABLES WHERE TABLE_SCHEMA NOT IN ('information_schema') AND ENGINE = 'MyISAM'").Scan(&row.value)
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

		rows, err := DbMetrics.db.Query("SELECT ENGINE,SUPPORT FROM information_schema.ENGINES ORDER BY ENGINE ASC")
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

		rows, err = DbMetrics.db.Query("SELECT ENGINE, IFNULL(SUM(DATA_LENGTH+INDEX_LENGTH),0), IFNULL(COUNT(ENGINE),0), IFNULL(SUM(DATA_LENGTH),0), IFNULL(SUM(INDEX_LENGTH),0) FROM information_schema.TABLES WHERE TABLE_SCHEMA NOT IN ('information_schema', 'performance_schema', 'mysql') AND ENGINE IS NOT NULL  GROUP BY ENGINE ORDER BY ENGINE ASC")
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
			output[engine_db] = MapJoin(output[engine_db], engine_elem)
		}
		metrics.DB.Metrics.Engine = output
		rows.Close()
	}

	DbMetrics.logger.Debug("collectMetrics ", metrics.DB.Metrics)
	return nil

}
