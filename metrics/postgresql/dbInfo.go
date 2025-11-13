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

type PgInfoBaseGatherer struct {
	logger        logging.Logger
	configuration *config.Config
}

func NewPgInfoBaseGatherer(logger logging.Logger, configuration *config.Config) *PgInfoBaseGatherer {
	return &PgInfoBaseGatherer{
		logger:        logger,
		configuration: configuration,
	}
}

func (PgInfoBase *PgInfoBaseGatherer) GetMetrics(metrics *models.Metrics) error {
	defer utils.HandlePanic(PgInfoBase.configuration, PgInfoBase.logger)

	var dbversion string
	info := make(models.MetricGroupValue)

	// PostgreSQL version
	err := models.DB.QueryRow("SELECT version()").Scan(&dbversion)
	if err != nil {
		PgInfoBase.logger.Error(err)
		return err
	}
	outStr := strings.Split(dbversion, " ")
	verStr := strings.Split(outStr[1], ".")
	pg_version := verStr[0] + "." + verStr[1]

	info["Version"] = pg_version
	err = os.WriteFile(PgInfoBase.configuration.ReleemConfDir+PostgreSQLVersionFile(), []byte(pg_version), 0644)
	if err != nil {
		PgInfoBase.logger.Error("WriteFile: Error write to file: ", err)
	}

	// PostgreSQL memory limit (from configuration)
	info["MemoryLimit"] = PgInfoBase.configuration.GetMemoryLimit()
	info["Type"] = "postgresql"
	metrics.DB.Info = info
	PgInfoBase.logger.V(5).Info("CollectMetrics PgInfoBase ", info)

	return nil
}

func PostgreSQLVersionFile() string {
	switch runtime.GOOS {
	case "windows":
		return "\\PostgreSQLVersion.txt"
	default: // для Linux и других UNIX-подобных систем
		return "/postgresql_version"
	}
}
