package postgresql

import "strings"

func pgStatStatementsQualifiedRelation(schema, relation string) string {
	return quotePgIdentifier(schema) + "." + quotePgIdentifier(relation)
}

func quotePgIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

var PG_STAT_VIEWS = []string{
	"pg_stat_archiver", "pg_stat_bgwriter", "pg_stat_database",
	"pg_stat_database_conflicts", "pg_stat_checkpointer",
}

var PG_STAT_PER_DB_VIEWS = map[string]string{
	"pg_stat_user_tables":    pgUserTableStatisticsQuery,
	"pg_statio_user_tables":  "SELECT * FROM pg_statio_user_tables",
	"pg_stat_user_indexes":   "SELECT * FROM pg_stat_user_indexes",
	"pg_statio_user_indexes": "SELECT * FROM pg_statio_user_indexes",
}

// Preserve the fields consumed by Platform's PostgreSQL optimizer without
// grouping all pg_class/pg_index rows as pg_stat_user_tables does.
const pgUserTableStatisticsQuery = `
SELECT c.oid AS relid, n.nspname AS schemaname, c.relname,
       pg_catalog.pg_stat_get_numscans(c.oid) AS seq_scan,
       i.idx_scan,
       pg_catalog.pg_stat_get_live_tuples(c.oid) AS n_live_tup,
       pg_catalog.pg_stat_get_dead_tuples(c.oid) AS n_dead_tup,
       pg_catalog.pg_stat_get_vacuum_count(c.oid) AS vacuum_count,
       pg_catalog.pg_stat_get_autovacuum_count(c.oid) AS autovacuum_count,
       pg_catalog.pg_stat_get_last_vacuum_time(c.oid) AS last_vacuum,
       pg_catalog.pg_stat_get_last_autovacuum_time(c.oid) AS last_autovacuum,
       pg_catalog.pg_stat_get_mod_since_analyze(c.oid) AS n_mod_since_analyze,
       pg_catalog.pg_stat_get_last_analyze_time(c.oid) AS last_analyze,
       pg_catalog.pg_stat_get_last_autoanalyze_time(c.oid) AS last_autoanalyze
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
LEFT JOIN LATERAL (
    SELECT SUM(pg_catalog.pg_stat_get_numscans(indexrelid))::bigint AS idx_scan
    FROM pg_catalog.pg_index
    WHERE indrelid = c.oid
) i ON true
WHERE c.relkind IN ('r', 't', 'm', 'p')
  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
  AND n.nspname !~ '^pg_toast'`
