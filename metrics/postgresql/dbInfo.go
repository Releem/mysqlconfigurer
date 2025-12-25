package postgresql

import (
	"os"
	"runtime"
	"strings"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
	logging "github.com/google/logger"
)

type DBInfoBaseGatherer struct {
	logger        logging.Logger
	configuration *config.Config
}

func NewDBInfoBaseGatherer(logger logging.Logger, configuration *config.Config) *DBInfoBaseGatherer {
	return &DBInfoBaseGatherer{
		logger:        logger,
		configuration: configuration,
	}
}

func (DBInfoBase *DBInfoBaseGatherer) GetMetrics(metrics *models.Metrics) error {
	defer utils.HandlePanic(DBInfoBase.configuration, DBInfoBase.logger)

	var dbversion string
	info := make(models.MetricGroupValue)

	// PostgreSQL version
	err := models.DB.QueryRow("SELECT version()").Scan(&dbversion)
	if err != nil {
		DBInfoBase.logger.Error(err)
		return err
	}
	outStr := strings.Split(dbversion, " ")
	verStr := strings.Split(outStr[1], ".")
	pg_version := verStr[0] + "." + verStr[1]

	info["Version"] = pg_version
	err = os.WriteFile(DBInfoBase.configuration.ReleemConfDir+PostgreSQLVersionFile(), []byte(pg_version), 0644)
	if err != nil {
		DBInfoBase.logger.Error("WriteFile: Error write to file: ", err)
	}

	// PostgreSQL memory limit (from configuration)
	info["MemoryLimit"] = DBInfoBase.configuration.GetMemoryLimit()
	info["Type"] = "postgresql"
	metrics.DB.Info = info
	DBInfoBase.logger.V(5).Info("CollectMetrics DBInfoBase ", info)

	return nil
}

func PostgreSQLVersionFile() string {
	switch runtime.GOOS {
	case "windows":
		return "\\DB_Version.txt"
	default: // для Linux и других UNIX-подобных систем
		return "/db_version"
	}
}
