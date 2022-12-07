package metrics

import (
	"database/sql"

	"github.com/Releem/mysqlconfigurer/releem-agent/config"
	"github.com/advantageous/go-logback/logging"
)

type MysqlEngineMetricsGatherer struct {
	logger logging.Logger
	debug  bool
	db     *sql.DB
}

func NewMysqlEngineMetricsGatherer(logger logging.Logger, db *sql.DB, configuration *config.Config) *MysqlEngineMetricsGatherer {

	if logger == nil {
		if configuration.Debug {
			logger = logging.NewSimpleDebugLogger("engine")
		} else {
			logger = logging.NewSimpleLogger("engine")
		}
	}

	return &MysqlEngineMetricsGatherer{
		logger: logger,
		debug:  configuration.Debug,
		db:     db,
	}
}

func (engine *MysqlEngineMetricsGatherer) GetMetrics() (Metric, error) {

	output := make(map[string]interface{})
	var engine_db, size, count, dsize, isize, engineenabled string

	rows, err := engine.db.Query("SELECT ENGINE,SUPPORT FROM information_schema.ENGINES ORDER BY ENGINE ASC")
	if err != nil {
		engine.logger.Error(err)
		metrics := Metric{"Engine": output}
		engine.logger.Debug("collectMetrics ", output)
		return metrics, nil
	}
	defer rows.Close()

	for rows.Next() {
		if err := rows.Scan(&engine_db, &engineenabled); err != nil {
			engine.logger.Error(err)
		}
		output[engine_db] = map[string]string{"Enabled": engineenabled}
	}
	rows.Close()
	rows, err = engine.db.Query("SELECT ENGINE, SUM(DATA_LENGTH+INDEX_LENGTH), COUNT(ENGINE), SUM(DATA_LENGTH), SUM(INDEX_LENGTH) FROM information_schema.TABLES WHERE TABLE_SCHEMA NOT IN ('information_schema', 'performance_schema', 'mysql') AND ENGINE IS NOT NULL GROUP BY ENGINE ORDER BY ENGINE ASC")
	if err != nil {
		engine.logger.Error(err)
		metrics := Metric{"Engine": output}
		engine.logger.Debug("collectMetrics ", output)
		return metrics, nil
	}
	defer rows.Close()
	for rows.Next() {
		if err := rows.Scan(&engine_db, &size, &count, &dsize, &isize); err != nil {
			engine.logger.Error(err)
		}

		engine_elem := make(map[string]string)
		_, found := output[engine_db]
		if found {
			engine_elem = output[engine_db].(map[string]string)
		}
		engine_elem["Table Number"] = count
		engine_elem["Total Size"] = size
		engine_elem["Data Size"] = dsize
		engine_elem["Index Size"] = isize

		output[engine_db] = engine_elem
	}
	metrics := Metric{"Engine": output}
	engine.logger.Debug("collectMetrics ", output)
	return metrics, nil

}
