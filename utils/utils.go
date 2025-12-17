// Example of a daemon with echo service
package utils

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/Releem/mysqlconfigurer/config"
	e "github.com/Releem/mysqlconfigurer/errors"
	"github.com/Releem/mysqlconfigurer/models"
	_ "github.com/go-sql-driver/mysql"
	logging "github.com/google/logger"
	_ "github.com/lib/pq"
	"github.com/pkg/errors"
)

func ProcessRepeaters(metrics *models.Metrics, repeaters models.MetricsRepeater,
	configuration *config.Config, logger logging.Logger, Mode models.ModeType) interface{} {
	defer HandlePanic(configuration, logger)

	result, err := repeaters.ProcessMetrics(configuration, *metrics, Mode)
	if err != nil {
		logger.Error("Repeater failed ", err)
	}
	return result
}

func CollectMetrics(gatherers []models.MetricsGatherer, logger logging.Logger, configuration *config.Config) *models.Metrics {
	defer HandlePanic(configuration, logger)
	var metrics models.Metrics
	for _, g := range gatherers {
		err := g.GetMetrics(&metrics)
		if err != nil {
			logger.Error("Problem getting metrics from gatherer")
			return nil
		}
	}
	return &metrics
}

func HandlePanic(configuration *config.Config, logger logging.Logger) {
	if r := recover(); r != nil {
		err := errors.WithStack(fmt.Errorf("%v", r))
		logger.Infof("%+v", err)
		sender := e.NewReleemErrorsRepeater(configuration, logger)
		sender.ProcessErrors(fmt.Sprintf("%+v", err))
	}
}

func MapJoin(map1, map2 models.MetricGroupValue) models.MetricGroupValue {
	for k, v := range map2 {
		map1[k] = v
	}
	return map1
}

func IsPath(path string, logger logging.Logger) bool {
	result_path := strings.Index(path, "/")
	logger.Info("DEBUG: IsPath ", path, " ", result_path)
	if result_path == 0 {
		return true
	} else {
		return false
	}
}

func ConnectionDatabase(configuration *config.Config, logger logging.Logger, DBname string) *sql.DB {
	dbType := configuration.GetDatabaseType()

	switch dbType {
	case "postgresql":
		if DBname == "" {
			DBname = "postgres"
		}
		return ConnectionPostgreSQL(configuration, logger, DBname)
	case "mysql":
		fallthrough
	default:
		if DBname == "" {
			DBname = "mysql"
		}
		return ConnectionMySQL(configuration, logger, DBname)
	}
}

func ConnectionMySQL(configuration *config.Config, logger logging.Logger, DBname string) *sql.DB {
	var db *sql.DB
	var err error
	var TypeConnection, MysqlSslMode string

	if configuration.MysqlSslMode {
		MysqlSslMode = "?tls=skip-verify"
	} else {
		MysqlSslMode = ""
	}
	if IsPath(configuration.MysqlHost, logger) {
		db, err = sql.Open("mysql", configuration.MysqlUser+":"+configuration.MysqlPassword+"@unix("+configuration.MysqlHost+")/"+DBname)
		TypeConnection = "unix"

	} else {
		db, err = sql.Open("mysql", configuration.MysqlUser+":"+configuration.MysqlPassword+"@tcp("+configuration.MysqlHost+":"+configuration.MysqlPort+")/"+DBname+MysqlSslMode)
		TypeConnection = "tcp"
	}
	if err != nil {
		logger.Error("Connection opening to failed ", err)
	}

	err = db.Ping()
	if err != nil {
		switch TypeConnection {
		case "unix":
			logger.Info("Connection failed to DB ", DBname, " via unix socket ", configuration.MysqlHost)
		case "tcp":
			logger.Info("Connection failed to DB ", DBname, " via tcp ", configuration.MysqlHost)
		}
	} else {
		switch TypeConnection {
		case "unix":
			logger.Info("Connection successful to DB ", DBname, " via unix socket ", configuration.MysqlHost)
		case "tcp":
			logger.Info("Connection successful to DB ", DBname, " via tcp ", configuration.MysqlHost)
		}
	}
	return db
}

func ConnectionPostgreSQL(configuration *config.Config, logger logging.Logger, DBname string) *sql.DB {
	var db *sql.DB
	var err error

	// Build PostgreSQL connection string
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		configuration.PgHost, configuration.PgPort, configuration.PgUser,
		configuration.PgPassword, DBname, configuration.PgSslMode)

	db, err = sql.Open("postgres", connStr)
	if err != nil {
		logger.Error("PostgreSQL connection opening failed ", err)
	}

	err = db.Ping()
	if err != nil {
		logger.Info("PostgreSQL connection failed to DB ", DBname, " via tcp ", configuration.PgHost, ":", configuration.PgPort)
	} else {
		logger.Info("PostgreSQL connection successful to DB ", DBname, " via tcp ", configuration.PgHost, ":", configuration.PgPort)
	}
	return db
}

func EnableEventsStatementsConsumers(configuration *config.Config, logger logging.Logger, uptime_str string) uint64 {
	var count_setup_consumers uint64

	// Only applicable to MySQL
	if configuration.GetDatabaseType() != "mysql" {
		return 0
	}

	uptime, err := strconv.Atoi(uptime_str)
	if err != nil {
		logger.Error(err)
	}
	count_setup_consumers = 0
	if configuration.QueryOptimization && uptime < 120 {
		err := models.DB.QueryRow("SELECT count(name) FROM performance_schema.setup_consumers WHERE enabled = 'YES' AND name LIKE 'events_statements_%' AND name != 'events_statements_cpu'").Scan(&count_setup_consumers)
		if err != nil {
			logger.Error(err)
			count_setup_consumers = 0
		}
		logger.Info("DEBUG: Found enabled performance_schema statements consumers: ", count_setup_consumers)
		if count_setup_consumers < 3 && configuration.InstanceType == "aws/rds" {
			_, err := models.DB.Query("CALL releem.enable_events_statements_consumers()")
			if err != nil {
				logger.Error("Failed to enable events_statements consumers", err)
			} else {
				logger.Info("Enable events_statements_consumers")
			}
		}
	}

	return count_setup_consumers
}

// IsSchemaNameExclude checks if a schema name should be excluded from query optimization
func IsSchemaNameExclude(SchemaName string, DatabasesQueryOptimization string) bool {
	if DatabasesQueryOptimization == "" {
		return false
	}
	for _, DbName := range strings.Split(DatabasesQueryOptimization, `,`) {
		if SchemaName == DbName {
			return false
		}
	}
	return true
}

func GetPostgreSQLMetrics(rows *sql.Rows, logger logging.Logger) []models.MetricGroupValue {

	cols, err := rows.Columns()
	if err != nil {
		logger.Error(err)
	}
	var out []map[string]any

	for rows.Next() {
		// Готовим приёмники под каждую колонку
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		err := rows.Scan(ptrs...)
		if err != nil {
			logger.Error(err)
			return nil
		}

		row := make(map[string]any, len(cols))
		for i, col := range cols {
			v := values[i]
			switch vv := v.(type) {
			case []byte:
				row[col] = string(vv)
			case nil:
				row[col] = nil
			default:
				row[col] = vv // может быть nil, time.Time, int64, float64, bool и т.д.
			}

		}
		out = append(out, row)
	}
	// Convert []map[string]any to []models.MetricGroupValue
	processListConverted := make([]models.MetricGroupValue, len(out))
	for i, row := range out {
		processListConverted[i] = models.MetricGroupValue(row)
	}
	return processListConverted
}
