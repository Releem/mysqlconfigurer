package metrics

import (
	"os"
	"regexp"
	"runtime"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
	logging "github.com/google/logger"
)

type DbInfoBaseGatherer struct {
	logger        logging.Logger
	configuration *config.Config
}

func NewDbInfoBaseGatherer(logger logging.Logger, configuration *config.Config) *DbInfoBaseGatherer {
	return &DbInfoBaseGatherer{
		logger:        logger,
		configuration: configuration,
	}
}

func (DbInfoBase *DbInfoBaseGatherer) GetMetrics(metrics *models.Metrics) error {
	defer utils.HandlePanic(DbInfoBase.configuration, DbInfoBase.logger)

	var row models.MetricValue
	var mysql_version string

	info := make(models.MetricGroupValue)
	// Mysql version
	err := models.DB.QueryRow("select VERSION()").Scan(&row.Value)
	if err != nil {
		DbInfoBase.logger.Error(err)
		return nil
	}
	re := regexp.MustCompile(`(.*?)\-.*`)
	version := re.FindStringSubmatch(row.Value)
	if len(version) > 0 {
		mysql_version = version[1]
	} else {
		mysql_version = row.Value
	}
	info["Version"] = mysql_version
	err = os.WriteFile(DbInfoBase.configuration.ReleemConfDir+MysqlVersionFile(), []byte(mysql_version), 0644)
	if err != nil {
		DbInfoBase.logger.Error("WriteFile: Error write to file: ", err)
	}
	// Mysql force memory limit
	info["MemoryLimit"] = DbInfoBase.configuration.GetMemoryLimit()

	metrics.DB.Info = info
	DbInfoBase.logger.V(5).Info("CollectMetrics DbInfoBase ", info)

	return nil
}
func MysqlVersionFile() string {
	switch runtime.GOOS {
	case "windows":
		return "\\MysqlVersion.txt"
	default: // для Linux и других UNIX-подобных систем
		return "/mysql_version"
	}
}
