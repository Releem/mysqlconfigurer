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
		var output []MetricGroupValue
		var row MetricValue

		var digest string
		rows, err := DbMetricsBase.db.Query("SELECT CONCAT(IFNULL(schema_name, 'NULL'), '_', IFNULL(digest, 'NULL')) as queryid, count_star as calls, round(avg_timer_wait/1000000, 0) as avg_time_us FROM performance_schema.events_statements_summary_by_digest")
		if err != nil {
			if err != sql.ErrNoRows {
				DbMetricsBase.logger.Error(err)
			}
		} else {
			for rows.Next() {
				err := rows.Scan(&digest, &row.name, &row.value)
				if err != nil {
					DbMetricsBase.logger.Error(err)
					return err
				}
				digest := MetricGroupValue{"queryid": digest, "calls": row.name, "avg_time_us": row.value}
				output = append(output, digest)
			}
			metrics.DB.Metrics.Latency = output
		}
	}

	DbMetricsBase.logger.Debug("collectMetrics ", metrics.DB.Metrics)
	return nil

}
