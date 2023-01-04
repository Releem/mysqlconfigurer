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
	info := make(MetricGroupValue)
	// Mysql version
	err := DbInfo.db.QueryRow("select VERSION()").Scan(&row.value)
	if err != nil {
		DbInfo.logger.Error(err)
		return nil
	}
	re := regexp.MustCompile(`(.*?)\-.*`)
	mysql_version := re.FindStringSubmatch(row.value)
	if len(mysql_version) > 0 {
		info["Version"] = mysql_version[1]
	} else {
		info["Version"] = row.value
	}
	// Mysql force memory limit
	info["MemoryLimit"] = DbInfo.configuration.GetMemoryLimit()
	metrics.DB.Info = info

	DbInfo.logger.Debug("collectMetrics ", metrics.DB.Info)
	return nil

}
