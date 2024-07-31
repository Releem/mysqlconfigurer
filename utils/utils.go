// Example of a daemon with echo service
package utils

import (
	"database/sql"
	"strings"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/advantageous/go-logback/logging"
	_ "github.com/go-sql-driver/mysql"
)

func IsPath(path string, logger logging.Logger) bool {
	result_path := strings.Index(path, "/")
	if result_path == 0 {
		return true
	} else {
		return false
	}
}

func ConnectionDatabase(configuration *config.Config, logger logging.Logger, DBname string) *sql.DB {
	var db *sql.DB
	var err error
	var TypeConnection, MysqlSslMode string

	if configuration.MysqlSslMode {
		MysqlSslMode = "?tls=skip-verify"
	} else {
		MysqlSslMode = ""
	}
	if IsPath(configuration.MysqlHost, logger) {
		db, err = sql.Open("mysql", configuration.MysqlUser+":"+configuration.MysqlPassword+"@unix("+configuration.MysqlHost+")/"+DBname+MysqlSslMode)
		TypeConnection = "unix"

	} else {
		db, err = sql.Open("mysql", configuration.MysqlUser+":"+configuration.MysqlPassword+"@tcp("+configuration.MysqlHost+":"+configuration.MysqlPort+")/"+DBname+MysqlSslMode)
		TypeConnection = "tcp"
	}
	if err != nil {
		logger.PrintError("Connection opening to failed", err)
	}

	err = db.Ping()
	if err != nil {
		logger.PrintError("Connection failed", err)
	} else {
		if TypeConnection == "unix" {
			logger.Println("Connect Success to DB ", DBname, " via unix socket", configuration.MysqlHost)
		} else if TypeConnection == "tcp" {
			logger.Println("Connect Success to DB ", DBname, " via tcp", configuration.MysqlHost)
		}
	}
	return db
}
