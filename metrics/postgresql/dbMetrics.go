package postgresql

import (
	"database/sql"
	"strings"
	"time"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
	u "github.com/Releem/mysqlconfigurer/utils"
	logging "github.com/google/logger"
	"github.com/hashicorp/go-version"
)

var PG_STAT_VIEWS = []string{
	"pg_stat_archiver", "pg_stat_bgwriter", "pg_stat_database",
	"pg_stat_database_conflicts", "pg_stat_checkpointer",
}

var PG_STAT_PER_DB_VIEWS = []string{
	"pg_stat_user_tables", "pg_statio_user_tables",
	"pg_stat_user_indexes", "pg_statio_user_indexes",
}

var PG_STAT_STATEMENTS = `
SELECT
	COALESCE(d.datname, 'NULL') as datname,
	s.queryid as queryid,
	sum(s.calls) AS calls,
	sum(s.total_exec_time) AS total_exec_time,
	sum(s.total_exec_time) / sum(s.calls) AS mean_exec_time
FROM pg_stat_statements s
LEFT JOIN pg_database d ON d.oid = s.dbid
GROUP BY d.datname, s.queryid
`

var PG_STAT_STATEMENTS_OLD_VERSION = `
SELECT
	COALESCE(d.datname, 'NULL') as datname,
	s.queryid::text as queryid,
	sum(s.calls) AS calls,
	sum(s.total_time) AS total_exec_time,
	sum(s.total_time) / sum(s.calls) AS mean_exec_time
FROM pg_stat_statements s
LEFT JOIN pg_database d ON d.oid = s.dbid
GROUP BY d.datname, s.queryid
`

type DBMetricsBaseGatherer struct {
	logger        logging.Logger
	configuration *config.Config
}

func NewDBMetricsBaseGatherer(logger logging.Logger, configuration *config.Config) *DBMetricsBaseGatherer {
	return &DBMetricsBaseGatherer{
		logger:        logger,
		configuration: configuration,
	}
}

type DBMetricsConfigGatherer struct {
	logger        logging.Logger
	configuration *config.Config
}

func NewDBMetricsConfigGatherer(logger logging.Logger, configuration *config.Config) *DBMetricsConfigGatherer {
	return &DBMetricsConfigGatherer{
		logger:        logger,
		configuration: configuration,
	}
}

type DBMetricsGatherer struct {
	logger        logging.Logger
	configuration *config.Config
}

func NewDBMetricsGatherer(logger logging.Logger, configuration *config.Config) *DBMetricsGatherer {
	return &DBMetricsGatherer{
		logger:        logger,
		configuration: configuration,
	}
}

func (DBMetricsBase *DBMetricsBaseGatherer) GetMetrics(metrics *models.Metrics) error {
	defer utils.HandlePanic(DBMetricsBase.configuration, DBMetricsBase.logger)
	{
		// Check if pg_stat_statements extension is available
		err := models.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')").Scan(&models.PgStatStatementsEnabled)
		if err != nil {
			DBMetricsBase.logger.Error("Error checking pg_stat_statements extension: ", err)
		}
	}
	{
		pg_stat := make(models.MetricGroupValue)
		// ver_current, _ := version.NewVersion(metrics.DB.Info["Version"].(string))
		// ver_postgresql, _ := version.NewVersion("9.4")
		// Collect DBMS internal metrics
		// pgStatViews := PG_STAT_VIEWS
		// if ver_current.LessThan(ver_postgresql) {
		// 	pgStatViews = PG_STAT_VIEWS_OLD_VERSION
		// }
		for _, view := range PG_STAT_VIEWS {
			rows, err := models.DB.Query(`
			SELECT * FROM ` + view)
			if err != nil {
				if strings.Contains(err.Error(), "relation \""+view+"\" does not exist") {
					DBMetricsBase.logger.Error(err)
				} else {
					DBMetricsBase.logger.Error(err)
				}
			} else {
				defer rows.Close()
				var existing []models.MetricGroupValue
				if val, ok := pg_stat[view].([]models.MetricGroupValue); ok {
					existing = val
				}
				pg_stat[view] = append(existing, utils.GetPostgreSQLMetrics(rows, DBMetricsBase.logger)...)
			}
		}

		// PostgreSQL Connection Statistics
		{
			var total_connections, active_connections, idle_connections string
			output := make(models.MetricGroupValue)

			err := models.DB.QueryRow(`
			SELECT 
				COUNT(*) as total_connections,
				COUNT(CASE WHEN state = 'active' THEN 1 END) as active_connections,
				COUNT(CASE WHEN state = 'idle' THEN 1 END) as idle_connections
			FROM pg_stat_activity`).Scan(&total_connections, &active_connections, &idle_connections)

			if err != nil {
				if err != sql.ErrNoRows {
					DBMetricsBase.logger.Error(err)
				}
			} else {
				output["total_connections"] = total_connections
				output["active_connections"] = active_connections
				output["idle_connections"] = idle_connections
			}
			pg_stat["PgConnections"] = output
		}
		// PostgreSQL Uptime Statistics
		{
			var uptime, timestamp string
			err := models.DB.QueryRow("SELECT EXTRACT(EPOCH FROM (now() - pg_postmaster_start_time()))::bigint AS uptime, EXTRACT(EPOCH FROM (now()) )::bigint AS timestamp").Scan(&uptime, &timestamp)
			if err != nil {
				DBMetricsBase.logger.Error(err)
			}
			pg_stat["Uptime"] = uptime
			pg_stat["timestamp"] = timestamp
		}
		metrics.DB.Metrics.Status = pg_stat
	}
	// List of databases
	{
		var database string
		var output []string
		rows, err := models.DB.Query("SELECT datname FROM pg_database WHERE datistemplate = false ORDER BY datname")
		if err != nil {
			DBMetricsBase.logger.Error(err)
			return err
		}
		defer rows.Close()

		for rows.Next() {
			err := rows.Scan(&database)
			if err != nil {
				DBMetricsBase.logger.Error(err)
				return err
			}
			output = append(output, database)
		}
		metrics.DB.Metrics.Databases = output
	}
	// Query latency from pg_stat_statements if available
	{
		var dealloc uint64
		var stats_reset string
		if models.PgStatStatementsEnabled {
			err := models.DB.QueryRow("SELECT dealloc, stats_reset FROM pg_stat_statements_info").Scan(&dealloc, &stats_reset)
			if err != nil {
				if strings.Contains(err.Error(), "relation \"pg_stat_statements_info\" does not exist") {
					DBMetricsBase.logger.Error(err)
				} else {
					DBMetricsBase.logger.Error(err)
				}
			}
		}
		metrics.DB.Metrics.Status["pg_stat_statements_info"] = models.MetricGroupValue{
			"dealloc":     dealloc,
			"stats_reset": stats_reset,
		}
	}
	DBMetricsBase.logger.V(5).Info("CollectMetrics DBMetricsBase ", metrics.DB.Metrics)

	return nil
}

func (DBMetricsConfig *DBMetricsConfigGatherer) GetMetrics(metrics *models.Metrics) error {
	defer utils.HandlePanic(DBMetricsConfig.configuration, DBMetricsConfig.logger)

	output := make(map[string]models.MetricGroupValue)
	var total_tables, row, size, count uint64
	var table_type string
	i := 0

	// // Total tables count
	// err := models.DB.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema NOT IN ('information_schema', 'pg_catalog')").Scan(&row)
	// if err != nil {
	// 	DBMetricsConfig.logger.Error(err)
	// }
	// total_tables += row

	// // PostgreSQL table engine statistics (PostgreSQL doesn't have engines like MySQL, but we can collect table types)
	// // Switch to each database to get table statistics
	// rows, err := models.DB.Query(`
	// 				SELECT
	// 					t.table_type,
	// 					COUNT(*) as table_count,
	// 					COALESCE(SUM(pg_total_relation_size(c.oid)), 0) as total_size
	// 				FROM information_schema.tables t
	// 				LEFT JOIN pg_class c ON c.relname = t.table_name
	// 				LEFT JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = t.table_schema
	// 				WHERE t.table_schema NOT IN ('information_schema', 'pg_catalog')
	// 				GROUP BY t.table_type`)

	// if err != nil {
	// 	DBMetricsConfig.logger.Error(err)
	// } else {
	// 	for rows.Next() {
	// 		err := rows.Scan(&table_type, &count, &size)
	// 		if err != nil {
	// 			DBMetricsConfig.logger.Error(err)
	// 		}
	// 		if output[table_type] == nil {
	// 			output[table_type] = models.MetricGroupValue{"Table Number": uint64(0), "Total Size": uint64(0)}
	// 		}
	// 		output[table_type]["Table Number"] = output[table_type]["Table Number"].(uint64) + count
	// 		output[table_type]["Total Size"] = output[table_type]["Total Size"].(uint64) + size
	// 	}
	// 	rows.Close()
	// }

	for _, database := range metrics.DB.Metrics.Databases {
		// if database == "postgres" {
		// 	continue
		// }
		// Switch to each database to get table statistics
		db := u.ConnectionDatabase(DBMetricsConfig.configuration, DBMetricsConfig.logger, database)
		defer db.Close()

		for _, view := range PG_STAT_PER_DB_VIEWS {
			rows, err := db.Query(`
			SELECT * FROM ` + view)
			if err != nil {
				DBMetricsConfig.logger.Error(err)
				return err
			}
			defer rows.Close()
			var existing []models.MetricGroupValue
			if val, ok := metrics.DB.Metrics.Status[view].([]models.MetricGroupValue); ok {
				existing = val
			}
			metrics.DB.Metrics.Status[view] = append(existing, utils.GetPostgreSQLMetrics(rows, DBMetricsConfig.logger)...)
		}
		// Total tables count
		err := db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema NOT IN ('information_schema', 'pg_catalog')").Scan(&row)
		if err != nil {
			DBMetricsConfig.logger.Error(err)
		}
		total_tables += row

		// PostgreSQL table engine statistics (PostgreSQL doesn't have engines like MySQL, but we can collect table types)
		rows, err := db.Query(`
				SELECT 
					t.table_type,
					COUNT(*) as table_count,
					COALESCE(SUM(pg_total_relation_size(c.oid)), 0) as total_size
				FROM information_schema.tables t
				LEFT JOIN pg_class c ON c.relname = t.table_name
				LEFT JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = t.table_schema
				WHERE t.table_schema NOT IN ('information_schema', 'pg_catalog')
				GROUP BY t.table_type`)

		if err != nil {
			DBMetricsConfig.logger.Error(err)
		} else {
			for rows.Next() {
				err := rows.Scan(&table_type, &count, &size)
				if err != nil {
					DBMetricsConfig.logger.Error(err)
					continue
				}
				if output[table_type] == nil {
					output[table_type] = models.MetricGroupValue{"Table Number": uint64(0), "Total Size": uint64(0)}
				}
				output[table_type]["Table Number"] = output[table_type]["Table Number"].(uint64) + count
				output[table_type]["Total Size"] = output[table_type]["Total Size"].(uint64) + size
			}
			rows.Close()
		}
		i += 1
		if i%25 == 0 {
			time.Sleep(3 * time.Second)
		}
	}
	metrics.DB.Metrics.Engine = output
	metrics.DB.Metrics.TotalTables = total_tables

	// Query latency from pg_stat_statements if available
	{
		var count_statements uint64
		count_statements = 0
		if models.PgStatStatementsEnabled {
			err := models.DB.QueryRow("SELECT COUNT(*) FROM pg_stat_statements").Scan(&count_statements)
			if err != nil {
				if err != sql.ErrNoRows {
					DBMetricsConfig.logger.Error(err)
				}
			}
		}
		metrics.DB.Metrics.CountQueriesLatency = count_statements
	}

	DBMetricsConfig.logger.V(5).Info("CollectMetrics DBMetricsConfig ", metrics.DB.Metrics)
	return nil
}

func (DBMetrics *DBMetricsGatherer) GetMetrics(metrics *models.Metrics) error {
	defer utils.HandlePanic(DBMetrics.configuration, DBMetrics.logger)

	ver_current, _ := version.NewVersion(metrics.DB.Info["Version"].(string))
	ver_postgresql, _ := version.NewVersion("13")
	// Collect DBMS internal metrics
	pgStatStatements := PG_STAT_STATEMENTS
	if ver_current.LessThan(ver_postgresql) {
		pgStatStatements = PG_STAT_STATEMENTS_OLD_VERSION
	}

	// Query latency from pg_stat_statements if available
	{
		var output []models.MetricGroupValue
		var queryid, datname string
		var calls int
		var total_exec_time, mean_exec_time float64

		if models.PgStatStatementsEnabled {
			// Collect query statistics from pg_stat_statements
			rows, err := models.DB.Query(pgStatStatements)

			if err != nil {
				if err != sql.ErrNoRows {
					DBMetrics.logger.Error(err)
				}
			} else {
				defer rows.Close()

				for rows.Next() {
					err := rows.Scan(&datname, &queryid, &calls, &total_exec_time, &mean_exec_time)
					if err != nil {
						DBMetrics.logger.Error(err)
						return err
					}

					// Convert to microseconds for compatibility with MySQL metrics
					total_exec_time_us := total_exec_time * 1000
					mean_exec_time_us := mean_exec_time * 1000
					output = append(output, models.MetricGroupValue{
						"datname":            datname,
						"queryid":            queryid,
						"calls":              calls,
						"total_exec_time_us": total_exec_time_us,
						"mean_exec_time_us":  mean_exec_time_us,
					})
				}
			}
		}
		metrics.DB.Queries = output
	}

	// // Process list from pg_stat_activity
	// {
	// 	var output []models.MetricGroupValue
	// 	var datname, usename, application_name, client_addr, state, query string
	// 	var pid int
	// 	var query_start, state_change, backend_start string

	// 	rows, err := models.DB.Query(`
	// 		SELECT
	// 			pid,
	// 			COALESCE(datname, '') as datname,
	// 			COALESCE(usename, '') as usename,
	// 			COALESCE(application_name, '') as application_name,
	// 			COALESCE(client_addr::text, '') as client_addr,
	// 			COALESCE(state, '') as state,
	// 			COALESCE(query_start::text, '') as query_start,
	// 			COALESCE(state_change::text, '') as state_change,
	// 			COALESCE(backend_start::text, '') as backend_start,
	// 			COALESCE(query, '') as query
	// 		FROM pg_stat_activity
	// 		WHERE state IS NOT NULL
	// 		ORDER BY backend_start DESC`)

	// 	if err != nil {
	// 		if err != sql.ErrNoRows {
	// 			DBMetrics.logger.Error(err)
	// 		}
	// 	} else {
	// 		for rows.Next() {
	// 			err := rows.Scan(&pid, &datname, &usename, &application_name, &client_addr, &state, &query_start, &state_change, &backend_start, &query)
	// 			if err != nil {
	// 				DBMetrics.logger.Error(err)
	// 				return err
	// 			}
	// 			output = append(output, models.MetricGroupValue{
	// 				"pid":              pid,
	// 				"datname":          datname,
	// 				"usename":          usename,
	// 				"application_name": application_name,
	// 				"client_addr":      client_addr,
	// 				"state":            state,
	// 				"query_start":      query_start,
	// 				"state_change":     state_change,
	// 				"backend_start":    backend_start,
	// 				"query":            query,
	// 			})
	// 		}
	// 		rows.Close()
	// 	}
	// 	metrics.DB.Metrics.ProcessList = output
	// }

	DBMetrics.logger.V(5).Info("CollectMetrics DBMetrics  queries, ", len(metrics.DB.Metrics.ProcessList), " processes")

	return nil
}
