package metrics

import (
	"database/sql"
	"os"
	"regexp"

	"github.com/Releem/mysqlconfigurer/config"
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
	defer HandlePanic(DbInfo.configuration, DbInfo.logger)

	var row MetricValue
	var mysql_version string

	info := make(MetricGroupValue)
	// Mysql version
	err := DbInfo.db.QueryRow("select VERSION()").Scan(&row.value)
	if err != nil {
		DbInfo.logger.Error(err)
		return nil
	}
	re := regexp.MustCompile(`(.*?)\-.*`)
	version := re.FindStringSubmatch(row.value)
	if len(version) > 0 {
		mysql_version = version[1]
	} else {
		mysql_version = row.value
	}
	info["Version"] = mysql_version
	err = os.WriteFile(DbInfo.configuration.ReleemConfDir+"/mysql_version", []byte(mysql_version), 0644)
	if err != nil {
		DbInfo.logger.Error("WriteFile: Error write to file: ", err)
	}
	// Mysql force memory limit
	info["MemoryLimit"] = DbInfo.configuration.GetMemoryLimit()
	metrics.DB.Info = info

	DbInfo.logger.Debug("collectMetrics ", metrics.DB.Info)
	return nil

}
