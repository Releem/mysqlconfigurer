package postgresql

import (
	"database/sql"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
	logging "github.com/google/logger"
)

// pgDefaultDatabase is the database the agent connects to when none is named.
const pgDefaultDatabase = "postgres"

// acquireDefaultDatabase returns the pooled handle for the default database;
// the caller must call release. main pins that handle as models.DB, so a nil
// pool falls back to models.DB itself.
func acquireDefaultDatabase(connections *utils.DatabaseConnectionPool, configuration *config.Config, logger logging.Logger) (db *sql.DB, release func(), err error) {
	if connections == nil {
		return models.DB, func() {}, nil
	}
	return connections.Acquire(pgDefaultDatabase, func() (*sql.DB, error) {
		return utils.ConnectionDatabaseErr(configuration, logger, pgDefaultDatabase)
	})
}
