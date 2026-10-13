package postgresql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/metrics/tablesize"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
	logging "github.com/google/logger"
)

type DBMetricsBaseGatherer struct {
	logger        logging.Logger
	configuration *config.Config
	capabilities  *PostgresCapabilities
}

func NewDBMetricsBaseGatherer(logger logging.Logger, configuration *config.Config, capabilities *PostgresCapabilities) *DBMetricsBaseGatherer {
	return &DBMetricsBaseGatherer{
		logger:        logger,
		configuration: configuration,
		capabilities:  capabilities,
	}
}

type DBMetricsConfigGatherer struct {
	logger         logging.Logger
	configuration  *config.Config
	now            func() time.Time
	refreshTables  bool
	tableSizeCache *tablesize.Cache[map[string]models.MetricGroupValue]
}

// pgManagedInternalDatabaseByInstanceType maps managed providers to the
// maintenance database that they keep for themselves. These names remain
// valid customer database names on local PostgreSQL installations.
var pgManagedInternalDatabaseByInstanceType = map[string]string{
	"aws/rds":          "rdsadmin",
	"gcp/cloudsql":     "cloudsqladmin",
	"azure/postgresql": "azure_maintenance",
}

func isPGManagedInstanceType(instanceType string) bool {
	_, managed := pgManagedInternalDatabaseByInstanceType[strings.ToLower(strings.TrimSpace(instanceType))]
	return managed
}

func isPGManagedInternalDatabase(instanceType, database string) bool {
	internalDatabase, managed := pgManagedInternalDatabaseByInstanceType[strings.ToLower(strings.TrimSpace(instanceType))]
	return managed && internalDatabase == strings.ToLower(strings.TrimSpace(database))
}

// NewTableSizeCache creates a cache shared by full and minute gatherers.
func NewTableSizeCache() *tablesize.Cache[map[string]models.MetricGroupValue] {
	return tablesize.New(tablesize.CloneEngine, func(value map[string]models.MetricGroupValue) tablesize.Summary {
		return tablesize.Summary{TotalTables: tablesize.EngineTableCount(value), TotalSize: tablesize.EngineTotal(value)}
	})
}

func NewDBMetricsConfigGatherer(logger logging.Logger, configuration *config.Config, refreshTables bool, tableSizeCache *tablesize.Cache[map[string]models.MetricGroupValue]) *DBMetricsConfigGatherer {
	if tableSizeCache == nil {
		tableSizeCache = NewTableSizeCache()
	}
	return &DBMetricsConfigGatherer{logger: logger, configuration: configuration, refreshTables: refreshTables, tableSizeCache: tableSizeCache, now: time.Now}
}

func (DBMetricsBase *DBMetricsBaseGatherer) GetMetrics(metrics *models.Metrics) error {
	defer utils.HandlePanic(DBMetricsBase.configuration, DBMetricsBase.logger)
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
				if !strings.Contains(err.Error(), "relation \""+view+"\" does not exist") {
					DBMetricsBase.logger.Error(err)
				}
			} else {
				defer rows.Close()
				var existing []models.MetricGroupValue
				if val, ok := pg_stat[view].([]models.MetricGroupValue); ok {
					existing = val
				}
				pg_stat[view] = append(existing, utils.ScanRows(rows, DBMetricsBase.logger)...)
			}
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
		rows, err := models.DB.Query(`
			SELECT datname
			FROM pg_database
			WHERE datistemplate = false
			ORDER BY datname`)
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
			if isPGManagedInternalDatabase(DBMetricsBase.configuration.InstanceType, database) {
				DBMetricsBase.logger.V(5).Info("Skipping provider-managed database ", database)
				continue
			}
			output = append(output, database)
		}
		if err := errors.Join(rows.Err(), rows.Close()); err != nil {
			return err
		}
		metrics.DB.Metrics.Databases = output
	}

	// Query latency from pg_stat_statements if available
	{
		capabilities, err := DBMetricsBase.capabilities.Resolve(context.Background(), models.DB)
		if err != nil {
			DBMetricsBase.logger.Error("Unable to collect pg_stat_statements capabilities: ", err)
			metrics.DB.Queries = nil
		} else {
			var dealloc uint64
			var stats_reset string

			if capabilities.PgStatStatementsInfoRelation != "" {
				err = models.DB.QueryRow("SELECT dealloc, stats_reset FROM "+capabilities.PgStatStatementsInfoRelation).Scan(&dealloc, &stats_reset)
				if err != nil && err != sql.ErrNoRows {
					DBMetricsBase.logger.Error(err)
				}
			}
			metrics.DB.Metrics.Status["pg_stat_statements_info"] = models.MetricGroupValue{
				"dealloc":     dealloc,
				"stats_reset": stats_reset,
			}

			var count_statements uint64

			err = models.DB.QueryRow("SELECT COUNT(*) FROM " + capabilities.PgStatStatementsRelation).Scan(&count_statements)
			if err != nil {
				if err != sql.ErrNoRows {
					DBMetricsBase.logger.Error(err)
				}
			}
			metrics.DB.Metrics.CountQueriesLatency = count_statements

			statementRows, err := collectPostgresQueryStats(context.Background(), models.DB, capabilities)
			if err != nil {
				DBMetricsBase.capabilities.InvalidateForError(err)
				if err != sql.ErrNoRows {
					DBMetricsBase.logger.Error(err)
				}
			} else {
				metrics.DB.Queries = postgresQueryStatsLegacyMetrics(statementRows)
			}
		}
	}

	// Process list from pg_stat_activity
	{
		var output []models.MetricGroupValue
		rows, err := models.DB.Query(`
			SELECT pid,
			datname,
			usename,
			COALESCE(host(client_addr), 'local') || COALESCE(':' || client_port::text, '') AS client_address,
			application_name,
			backend_type,
			state,
			CONCAT_WS(' ', wait_event_type::text, wait_event::text) AS wait_event,
			EXTRACT(EPOCH FROM (now() - query_start))::bigint AS query_time,
			EXTRACT(EPOCH FROM (now() - xact_start))::bigint AS tx_time,
			EXTRACT(EPOCH FROM (now() - state_change))::bigint AS state_time,
			query
			FROM pg_stat_activity
			WHERE state IS NOT NULL
			ORDER BY pid`)
		if err != nil {
			if err != sql.ErrNoRows {
				DBMetricsBase.logger.Error(err)
			}
		} else {
			defer rows.Close()
			output = append(output, utils.ScanRows(rows, DBMetricsBase.logger)...)
		}

		metrics.DB.Metrics.ProcessList = output
	}

	DBMetricsBase.logger.V(5).Info("CollectMetrics DBMetricsBase ", metrics.DB.Metrics)

	return nil
}

func (g *DBMetricsConfigGatherer) GetMetrics(metrics *models.Metrics) error {
	defer utils.HandlePanic(g.configuration, g.logger)
	// Per-database activity remains live even when table sizes are cached.
	for _, database := range metrics.DB.Metrics.Databases {
		db := utils.ConnectionDatabase(g.configuration, g.logger, database)
		if db == nil {
			g.logger.Error("Connection to database failed: ", database)
			continue
		}
		for view, query := range PG_STAT_PER_DB_VIEWS {
			rows, err := db.Query(query)
			if err != nil {
				g.logger.Error(err)
				continue
			}
			existing, _ := metrics.DB.Metrics.Status[view].([]models.MetricGroupValue)
			metrics.DB.Metrics.Status[view] = append(existing, utils.ScanRows(rows, g.logger)...)
			rows.Close()
		}
		if err := db.Close(); err != nil {
			g.logger.Error(err)
		}
	}
	snapshot, present := g.tableSizeCache.Peek()
	if g.refreshTables {
		input := tablesize.InputFromMetrics(metrics, g.configuration, g.now())
		result, err := g.tableSizeCache.GetOrRefresh(input, func() (map[string]models.MetricGroupValue, error) {
			return g.collectTableSizes(metrics.DB.Metrics.Databases)
		})
		if err != nil {
			g.logger.Error(err)
			return err
		}
		if result.UsedStale {
			g.logger.Warningf("using stale table size metrics after refresh error: %v", result.RefreshErr)
		}
		snapshot, present = result.Value, true
	}
	if present {
		metrics.DB.Metrics.Engine = snapshot
		metrics.DB.Metrics.TotalTables = tablesize.EngineTableCount(snapshot)
	}
	g.logger.V(5).Info("CollectMetrics DBMetricsConfig ", metrics.DB.Metrics)
	return nil
}

func (g *DBMetricsConfigGatherer) collectTableSizes(databases []string) (map[string]models.MetricGroupValue, error) {
	snapshot := make(map[string]models.MetricGroupValue)
	for i, database := range databases {
		db := utils.ConnectionDatabase(g.configuration, g.logger, database)
		if db == nil {
			return nil, fmt.Errorf("connect to database %q for table sizes", database)
		}
		rows, err := db.Query(`
  SELECT t.table_type, COUNT(*) as table_count,
   COALESCE(SUM(pg_total_relation_size(c.oid)), 0) as total_size
  FROM information_schema.tables t
  JOIN pg_namespace n ON n.nspname = t.table_schema
  JOIN pg_class c ON c.relnamespace = n.oid AND c.relname = t.table_name
  WHERE t.table_schema NOT IN ('information_schema', 'pg_catalog')
  GROUP BY t.table_type`)
		if err != nil {
			return nil, fmt.Errorf("collect table sizes in database %q: %w", database, errors.Join(err, db.Close()))
		}
		for rows.Next() {
			var tableType string
			var count, size uint64
			if err := rows.Scan(&tableType, &count, &size); err != nil {
				return nil, fmt.Errorf("collect table sizes in database %q: %w", database, errors.Join(err, rows.Err(), rows.Close(), db.Close()))
			}
			if snapshot[tableType] == nil {
				snapshot[tableType] = models.MetricGroupValue{"Table Number": uint64(0), "Total Size": uint64(0)}
			}
			snapshot[tableType]["Table Number"] = snapshot[tableType]["Table Number"].(uint64) + count
			snapshot[tableType]["Total Size"] = snapshot[tableType]["Total Size"].(uint64) + size
		}
		if err := errors.Join(rows.Err(), rows.Close(), db.Close()); err != nil {
			return nil, fmt.Errorf("collect table sizes in database %q: %w", database, err)
		}
		if (i+1)%25 == 0 {
			time.Sleep(3 * time.Second)
		}
	}
	return snapshot, nil
}
