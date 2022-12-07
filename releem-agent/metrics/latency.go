package metrics

import (
	"database/sql"

	"github.com/Releem/mysqlconfigurer/releem-agent/config"
	"github.com/advantageous/go-logback/logging"
)

type MysqlLatencyMetricsGatherer struct {
	logger logging.Logger
	debug  bool
	db     *sql.DB
}

func NewMysqlLatencyMetricsGatherer(logger logging.Logger, db *sql.DB, configuration *config.Config) *MysqlLatencyMetricsGatherer {

	if logger == nil {
		if configuration.Debug {
			logger = logging.NewSimpleDebugLogger("Latency")
		} else {
			logger = logging.NewSimpleLogger("Latency")
		}
	}

	return &MysqlLatencyMetricsGatherer{
		logger: logger,
		debug:  configuration.Debug,
		db:     db,
	}
}

func (latency *MysqlLatencyMetricsGatherer) GetMetrics() (Metric, error) {

	output := make(map[string]interface{})

	rows, err := latency.db.Query("select 'Latency' as name, `s2`.`avg_us` AS `avg_us` from ((select count(0) AS `cnt`,round(`performance_schema`.`events_statements_summary_by_digest`.`AVG_TIMER_WAIT` / 1000000,0) AS `avg_us` from `performance_schema`.`events_statements_summary_by_digest` group by round(`performance_schema`.`events_statements_summary_by_digest`.`AVG_TIMER_WAIT` / 1000000,0)) `s1` join (select count(0) AS `cnt`,round(`performance_schema`.`events_statements_summary_by_digest`.`AVG_TIMER_WAIT` / 1000000,0) AS `avg_us` from `performance_schema`.`events_statements_summary_by_digest` group by round(`performance_schema`.`events_statements_summary_by_digest`.`AVG_TIMER_WAIT` / 1000000,0)) `s2` on(`s1`.`avg_us` <= `s2`.`avg_us`)) group by `s2`.`avg_us` having ifnull(sum(`s1`.`cnt`) / nullif((select count(0) from `performance_schema`.`events_statements_summary_by_digest`),0),0) > 0.95 order by ifnull(sum(`s1`.`cnt`) / nullif((select count(0) from `performance_schema`.`events_statements_summary_by_digest`),0),0) limit 1")
	if err != nil {
		latency.logger.Error(err)
		metrics := Metric{"Metrics": output}
		latency.logger.Debug("collectMetrics ", output)
		return metrics, nil
	}
	defer rows.Close()

	for rows.Next() {
		var row MetricValue
		if err := rows.Scan(&row.name, &row.value); err != nil {
			latency.logger.Error(err)
		}
		output[row.name] = row.value
	}
	metrics := Metric{"Metrics": output}
	latency.logger.Debug("collectMetrics ", output)
	return metrics, nil

}
