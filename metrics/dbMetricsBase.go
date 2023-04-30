package metrics

import (
	"database/sql"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/advantageous/go-logback/logging"
)

type DbMetricsBaseGatherer struct {
	logger logging.Logger
	debug  bool
	db     *sql.DB
}

func NewDbMetricsBaseGatherer(logger logging.Logger, db *sql.DB, configuration *config.Config) *DbMetricsBaseGatherer {

	if logger == nil {
		if configuration.Debug {
			logger = logging.NewSimpleDebugLogger("DbMetricsBase")
		} else {
			logger = logging.NewSimpleLogger("DbMetricsBase")
		}
	}

	return &DbMetricsBaseGatherer{
		logger: logger,
		debug:  configuration.Debug,
		db:     db,
	}
}

func (DbMetricsBase *DbMetricsBaseGatherer) GetMetrics(metrics *Metrics) error {
	// Mysql Status
	{
		output := make(MetricGroupValue)
		var row MetricValue
		rows, err := DbMetricsBase.db.Query("SHOW STATUS")

		if err != nil {
			DbMetricsBase.logger.Error(err)
			return err
		}
		for rows.Next() {
			err := rows.Scan(&row.name, &row.value)
			if err != nil {
				DbMetricsBase.logger.Error(err)
				return err
			}
			output[row.name] = row.value
		}

		rows.Close()
		rows, err = DbMetricsBase.db.Query("SHOW GLOBAL STATUS")
		if err != nil {
			DbMetricsBase.logger.Error(err)
			return err
		}
		for rows.Next() {
			err := rows.Scan(&row.name, &row.value)
			if err != nil {
				DbMetricsBase.logger.Error(err)
				return err
			}
			output[row.name] = row.value
		}
		metrics.DB.Metrics.Status = output
		rows.Close()
	}
	// Latency
	{
		var row MetricValue
		err := DbMetricsBase.db.QueryRow("select `s2`.`avg_us` AS `avg_us` from ((select count(0) AS `cnt`,round(`performance_schema`.`events_statements_summary_by_digest`.`AVG_TIMER_WAIT` / 1000000,0) AS `avg_us` from `performance_schema`.`events_statements_summary_by_digest` group by round(`performance_schema`.`events_statements_summary_by_digest`.`AVG_TIMER_WAIT` / 1000000,0)) `s1` join (select count(0) AS `cnt`,round(`performance_schema`.`events_statements_summary_by_digest`.`AVG_TIMER_WAIT` / 1000000,0) AS `avg_us` from `performance_schema`.`events_statements_summary_by_digest` group by round(`performance_schema`.`events_statements_summary_by_digest`.`AVG_TIMER_WAIT` / 1000000,0)) `s2` on(`s1`.`avg_us` <= `s2`.`avg_us`)) group by `s2`.`avg_us` having ifnull(sum(`s1`.`cnt`) / nullif((select count(0) from `performance_schema`.`events_statements_summary_by_digest`),0),0) > 0.95 order by ifnull(sum(`s1`.`cnt`) / nullif((select count(0) from `performance_schema`.`events_statements_summary_by_digest`),0),0) limit 1").Scan(&row.value)
		if err != nil {
			if err != sql.ErrNoRows {
				DbMetricsBase.logger.Error(err)
			}
		} else {
			metrics.DB.Metrics.Latency = row.value
		}
	}

	DbMetricsBase.logger.Debug("collectMetrics ", metrics.DB.Metrics)
	return nil

}
