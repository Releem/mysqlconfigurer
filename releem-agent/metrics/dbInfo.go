package metrics

import (
	"database/sql"
	"regexp"

	"github.com/Releem/mysqlconfigurer/releem-agent/config"
	"github.com/advantageous/go-logback/logging"
)

type DbInfoGatherer struct {
	logger        logging.Logger
	configuration *config.Config
	db            *sql.DB
}

func NewDbInfoGatherer(logger logging.Logger, db *sql.DB, configuration *config.Config) *DbInfoGatherer {

	if logger == nil {
		if configuration.Debug {
			logger = logging.NewSimpleDebugLogger("DbInfo")
		} else {
			logger = logging.NewSimpleLogger("DbInfo")
		}
	}

	return &DbInfoGatherer{
		logger:        logger,
		configuration: configuration,
		db:            db,
	}
}

func (DbInfo *DbInfoGatherer) GetMetrics(metrics *Metrics) error {

	var row MetricValue
	info := make(map[string]interface{})

	err := DbInfo.db.QueryRow("select VERSION()").Scan(&row.value)
	if err != nil {
		DbInfo.logger.Error(err)
		return nil
	}
	re := regexp.MustCompile(`(.*?)\-.*`)
	info["Version"] = re.FindStringSubmatch(row.value)[1]
	info["MemoryLimit"] = DbInfo.configuration.GetMemoryLimit()
	metrics.DB.Info = info

	DbInfo.logger.Debug("collectMetrics ", metrics.DB.Info)
	return nil

}
