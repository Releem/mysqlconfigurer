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
	// Mysql Status
	{
		output := make(MetricGroupValue)
		var row MetricValue
		rows, err := DbMetrics.db.Query("SHOW STATUS")

		if err != nil {
			DbMetrics.logger.Error(err)
			return err
		}
		for rows.Next() {
			err := rows.Scan(&row.name, &row.value)
			if err != nil {
				DbMetrics.logger.Error(err)
				return err
			}
			output[row.name] = row.value
		}

		rows.Close()
		rows, err = DbMetrics.db.Query("SHOW GLOBAL STATUS")
		if err != nil {
			DbMetrics.logger.Error(err)
			return err
		}
		for rows.Next() {
			err := rows.Scan(&row.name, &row.value)
			if err != nil {
				DbMetrics.logger.Error(err)
				return err
			}
			output[row.name] = row.value
		}
		metrics.DB.Metrics.Status = output
		rows.Close()
	}
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
	//Latency
	{
		var row MetricValue
		err := DbMetrics.db.QueryRow("select `s2`.`avg_us` AS `avg_us` from ((select count(0) AS `cnt`,round(`performance_schema`.`events_statements_summary_by_digest`.`AVG_TIMER_WAIT` / 1000000,0) AS `avg_us` from `performance_schema`.`events_statements_summary_by_digest` group by round(`performance_schema`.`events_statements_summary_by_digest`.`AVG_TIMER_WAIT` / 1000000,0)) `s1` join (select count(0) AS `cnt`,round(`performance_schema`.`events_statements_summary_by_digest`.`AVG_TIMER_WAIT` / 1000000,0) AS `avg_us` from `performance_schema`.`events_statements_summary_by_digest` group by round(`performance_schema`.`events_statements_summary_by_digest`.`AVG_TIMER_WAIT` / 1000000,0)) `s2` on(`s1`.`avg_us` <= `s2`.`avg_us`)) group by `s2`.`avg_us` having ifnull(sum(`s1`.`cnt`) / nullif((select count(0) from `performance_schema`.`events_statements_summary_by_digest`),0),0) > 0.95 order by ifnull(sum(`s1`.`cnt`) / nullif((select count(0) from `performance_schema`.`events_statements_summary_by_digest`),0),0) limit 1").Scan(&row.value)
		if err != nil {
			if err != sql.ErrNoRows {
				DbMetrics.logger.Error(err)
			}
		} else {
			metrics.DB.Metrics.Latency = row.value
		}
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
		rows, err = DbMetrics.db.Query("SELECT ENGINE, SUM(DATA_LENGTH+INDEX_LENGTH), COUNT(ENGINE), SUM(DATA_LENGTH), SUM(INDEX_LENGTH) FROM information_schema.TABLES WHERE TABLE_SCHEMA NOT IN ('information_schema', 'performance_schema', 'mysql') AND ENGINE IS NOT NULL  GROUP BY ENGINE ORDER BY ENGINE ASC")
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
