package metrics

import (
	"database/sql"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/advantageous/go-logback/logging"
)

type DbCollectQueries struct {
	logger logging.Logger
	debug  bool
	db     *sql.DB
}

func NewDbCollectQueries(logger logging.Logger, db *sql.DB, configuration *config.Config) *DbCollectQueries {

	if logger == nil {
		if configuration.Debug {
			logger = logging.NewSimpleDebugLogger("DbCollectQueries")
		} else {
			logger = logging.NewSimpleLogger("DbCollectQueries")
		}
	}

	return &DbCollectQueries{
		logger: logger,
		debug:  configuration.Debug,
		db:     db,
	}
}

func (DbCollectQueries *DbCollectQueries) GetMetrics(metrics *Metrics) error {

	// Latency
	{
		var output []MetricGroupValue
		var schema_name, query string
		var calls, avg_time_us int

		rows, err := DbCollectQueries.db.Query("SELECT IFNULL(schema_name, 'NULL') as schema_name, IFNULL(digest_text, 'NULL') as query, count_star as calls, round(avg_timer_wait/1000000, 0) as avg_time_us FROM performance_schema.events_statements_summary_by_digest")
		if err != nil {
			if err != sql.ErrNoRows {
				DbCollectQueries.logger.Error(err)
			}
		} else {
			for rows.Next() {
				err := rows.Scan(&schema_name, &query, &calls, &avg_time_us)
				if err != nil {
					DbCollectQueries.logger.Error(err)
					return err
				}
				digest := MetricGroupValue{"schema_name": schema_name, "query": query, "calls": calls, "avg_time_us": avg_time_us}
				output = append(output, digest)
			}
		}
		if len(output) != 0 {
			metrics.DB.Queries = output
		} else {
			metrics.DB.Queries = nil
		}
	}

	DbCollectQueries.logger.Debug("collectMetrics ", metrics.DB.Queries)
	return nil

}
