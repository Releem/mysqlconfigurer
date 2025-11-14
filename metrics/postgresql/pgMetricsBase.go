package postgresql

import (
	"database/sql"
	"time"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
	logging "github.com/google/logger"
	"github.com/hashicorp/go-version"
)

var PG_STAT_VIEWS = []string{
	"pg_stat_archiver", "pg_stat_bgwriter", "pg_stat_database",
	"pg_stat_database_conflicts", "pg_stat_user_tables", "pg_statio_user_tables",
	"pg_stat_user_indexes", "pg_statio_user_indexes",
}

var PG_STAT_VIEWS_OLD_VERSION = []string{
	"pg_stat_bgwriter", "pg_stat_database",
	"pg_stat_database_conflicts", "pg_stat_user_tables", "pg_statio_user_tables",
	"pg_stat_user_indexes", "pg_statio_user_indexes",
}

type PgMetricsBaseGatherer struct {
	logger        logging.Logger
	configuration *config.Config
}

func NewPgMetricsBaseGatherer(logger logging.Logger, configuration *config.Config) *PgMetricsBaseGatherer {
	return &PgMetricsBaseGatherer{
		logger:        logger,
		configuration: configuration,
	}
}

type PgMetricsGatherer struct {
	logger        logging.Logger
	configuration *config.Config
}

func NewPgMetricsGatherer(logger logging.Logger, configuration *config.Config) *PgMetricsGatherer {
	return &PgMetricsGatherer{
		logger:        logger,
		configuration: configuration,
	}
}

type PgMetricsMetricsBaseGatherer struct {
	logger        logging.Logger
	configuration *config.Config
}

func NewPgMetricsMetricsBaseGatherer(logger logging.Logger, configuration *config.Config) *PgMetricsMetricsBaseGatherer {
	return &PgMetricsMetricsBaseGatherer{
		logger:        logger,
		configuration: configuration,
	}
}
func (PgMetricsBase *PgMetricsBaseGatherer) GetMetrics(metrics *models.Metrics) error {
	defer utils.HandlePanic(PgMetricsBase.configuration, PgMetricsBase.logger)
	pg_stat := make(models.MetricGroupValue)
	ver_current, _ := version.NewVersion(metrics.DB.Info["Version"].(string))
	ver_postgresql, _ := version.NewVersion("9.4")
	// Collect DBMS internal metrics
	pgStatViews := PG_STAT_VIEWS
	if ver_current.LessThan(ver_postgresql) {
		pgStatViews = PG_STAT_VIEWS_OLD_VERSION
	}
	for _, view := range pgStatViews {
		rows, err := models.DB.Query(`
			SELECT * FROM ` + view)
		if err != nil {
			PgMetricsBase.logger.Error(err)
			return err
		}
		defer rows.Close()
		pg_stat[view] = utils.GetPostgreSQLMetrics(rows, PgMetricsBase.logger)
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
				PgMetricsBase.logger.Error(err)
			}
		} else {
			output["total_connections"] = total_connections
			output["active_connections"] = active_connections
			output["idle_connections"] = idle_connections
		}
		pg_stat["PgConnections"] = output
	}

	metrics.DB.Metrics.Status = pg_stat

	// List of databases
	{
		var database string
		var output []string
		rows, err := models.DB.Query("SELECT datname FROM pg_database WHERE datistemplate = false ORDER BY datname")
		if err != nil {
			PgMetricsBase.logger.Error(err)
			return err
		}
		defer rows.Close()

		for rows.Next() {
			err := rows.Scan(&database)
			if err != nil {
				PgMetricsBase.logger.Error(err)
				return err
			}
			output = append(output, database)
		}
		metrics.DB.Metrics.Databases = output
	}

	// Total tables count
	{
		var row uint64
		err := models.DB.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema NOT IN ('information_schema', 'pg_catalog')").Scan(&row)
		if err != nil {
			PgMetricsBase.logger.Error(err)
			return err
		}
		metrics.DB.Metrics.TotalTables = row
	}
	{
		// Check if pg_stat_statements extension is available
		err := models.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')").Scan(&models.PgStatStatementsEnabled)
		if err != nil {
			PgMetricsBase.logger.Error("Error checking pg_stat_statements extension: ", err)
			return err
		}
	}
	PgMetricsBase.logger.V(5).Info("CollectMetrics PgMetricsBase ", metrics.DB.Metrics)

	return nil
}

func (PgMetrics *PgMetricsGatherer) GetMetrics(metrics *models.Metrics) error {
	defer utils.HandlePanic(PgMetrics.configuration, PgMetrics.logger)
	// PostgreSQL table engine statistics (PostgreSQL doesn't have engines like MySQL, but we can collect table types)
	{
		var table_type string
		var size, count uint64
		output := make(map[string]models.MetricGroupValue)

		// Initialize with common PostgreSQL table types
		output["BASE TABLE"] = models.MetricGroupValue{"Table Number": uint64(0), "Total Size": uint64(0)}
		output["VIEW"] = models.MetricGroupValue{"Table Number": uint64(0), "Total Size": uint64(0)}
		output["MATERIALIZED VIEW"] = models.MetricGroupValue{"Table Number": uint64(0), "Total Size": uint64(0)}

		i := 0
		for _, database := range metrics.DB.Metrics.Databases {
			// Switch to each database to get table statistics
			rows, err := models.DB.Query(`
				SELECT 
					t.table_type,
					COUNT(*) as table_count,
					COALESCE(SUM(pg_total_relation_size(c.oid)), 0) as total_size
				FROM information_schema.tables t
				LEFT JOIN pg_class c ON c.relname = t.table_name
				LEFT JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = t.table_schema
				WHERE t.table_catalog = $1 
				  AND t.table_schema NOT IN ('information_schema', 'pg_catalog')
				GROUP BY t.table_type`, database)

			if err != nil {
				PgMetrics.logger.Error(err)
				continue
			}

			for rows.Next() {
				err := rows.Scan(&table_type, &count, &size)
				if err != nil {
					PgMetrics.logger.Error(err)
					continue
				}

				if output[table_type] == nil {
					output[table_type] = models.MetricGroupValue{"Table Number": uint64(0), "Total Size": uint64(0)}
				}
				output[table_type]["Table Number"] = output[table_type]["Table Number"].(uint64) + count
				output[table_type]["Total Size"] = output[table_type]["Total Size"].(uint64) + size
			}
			rows.Close()

			i += 1
			if i%25 == 0 {
				time.Sleep(3 * time.Second)
			}
		}

		metrics.DB.Metrics.Engine = output

	}
	// Query latency from pg_stat_statements if available
	{
		var count_statements uint64
		count_statements = 0
		if models.PgStatStatementsEnabled {
			err := models.DB.QueryRow("SELECT COUNT(*) FROM pg_stat_statements").Scan(&count_statements)
			if err != nil {
				if err != sql.ErrNoRows {
					PgMetrics.logger.Error(err)
				}
			}
		}
		metrics.DB.Metrics.CountQueriesLatency = count_statements

	}
	PgMetrics.logger.V(5).Info("CollectMetrics PgMetrics ", metrics.DB.Metrics)

	return nil
}

func (PgMetricsMetricsBase *PgMetricsMetricsBaseGatherer) GetMetrics(metrics *models.Metrics) error {
	defer utils.HandlePanic(PgMetricsMetricsBase.configuration, PgMetricsMetricsBase.logger)

	// Query latency from pg_stat_statements if available
	{
		var output []models.MetricGroupValue
		var query_id, dbname string
		var calls int
		var total_time, mean_time float64

		if models.PgStatStatementsEnabled {
			// Collect query statistics from pg_stat_statements
			rows, err := models.DB.Query(`
				SELECT 
					COALESCE(d.datname, NULL) as dbname,
					s.queryid::text as query_id,
					s.calls,
					s.total_exec_time as total_time,
					s.mean_exec_time as mean_time
				FROM pg_stat_statements s
				LEFT JOIN pg_database d ON d.oid = s.dbid
				WHERE s.calls > 0
				ORDER BY s.total_exec_time DESC
				LIMIT 1000`)

			if err != nil {
				// Try older version of pg_stat_statements (pre-13)
				rows, err = models.DB.Query(`
					SELECT 
						COALESCE(d.datname, NULL) as dbname,
						s.queryid::text as query_id,
						s.calls,
						s.total_time as total_time,
						s.mean_time as mean_time
					FROM pg_stat_statements s
					LEFT JOIN pg_database d ON d.oid = s.dbid
					WHERE s.calls > 0
					ORDER BY s.total_time DESC
					LIMIT 1000`)

				if err != nil {
					if err != sql.ErrNoRows {
						PgMetricsMetricsBase.logger.Error(err)
					}
				}
			}
			defer rows.Close()

			for rows.Next() {
				err := rows.Scan(&dbname, &query_id, &calls, &total_time, &mean_time)
				if err != nil {
					PgMetricsMetricsBase.logger.Error(err)
					return err
				}

				// Convert to microseconds for compatibility with MySQL metrics
				total_time_us := int(total_time * 1000)
				mean_time_us := int(mean_time * 1000)
				output = append(output, models.MetricGroupValue{
					"schema_name": dbname,
					"query_id":    query_id,
					"calls":       calls,
					"avg_time_us": mean_time_us,
					"sum_time_us": total_time_us,
				})
			}
		}

		metrics.DB.Metrics.QueriesLatency = output
	}

	// Process list from pg_stat_activity
	{
		var output []models.MetricGroupValue
		var datname, usename, application_name, client_addr, state, query string
		var pid int
		var query_start, state_change, backend_start string

		rows, err := models.DB.Query(`
			SELECT 
				pid,
				COALESCE(datname, '') as datname,
				COALESCE(usename, '') as usename,
				COALESCE(application_name, '') as application_name,
				COALESCE(client_addr::text, '') as client_addr,
				COALESCE(state, '') as state,
				COALESCE(query_start::text, '') as query_start,
				COALESCE(state_change::text, '') as state_change,
				COALESCE(backend_start::text, '') as backend_start,
				COALESCE(query, '') as query
			FROM pg_stat_activity 
			WHERE state IS NOT NULL 
			ORDER BY backend_start DESC`)

		if err != nil {
			if err != sql.ErrNoRows {
				PgMetricsMetricsBase.logger.Error(err)
			}
		} else {
			for rows.Next() {
				err := rows.Scan(&pid, &datname, &usename, &application_name, &client_addr, &state, &query_start, &state_change, &backend_start, &query)
				if err != nil {
					PgMetricsMetricsBase.logger.Error(err)
					return err
				}
				output = append(output, models.MetricGroupValue{
					"pid":              pid,
					"datname":          datname,
					"usename":          usename,
					"application_name": application_name,
					"client_addr":      client_addr,
					"state":            state,
					"query_start":      query_start,
					"state_change":     state_change,
					"backend_start":    backend_start,
					"query":            query,
				})
			}
			rows.Close()
		}
		metrics.DB.Metrics.ProcessList = output
	}

	PgMetricsMetricsBase.logger.V(5).Info("CollectMetrics PgMetricsMetricsBase ", len(metrics.DB.Metrics.QueriesLatency), " queries, ", len(metrics.DB.Metrics.ProcessList), " processes")

	return nil
}
