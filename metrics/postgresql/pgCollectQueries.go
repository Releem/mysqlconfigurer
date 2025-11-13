package postgresql

import (
	"database/sql"
	"sort"
	"strings"
	"time"

	"github.com/Releem/mysqlconfigurer/models"
	u "github.com/Releem/mysqlconfigurer/utils"

	"github.com/Releem/mysqlconfigurer/config"
	logging "github.com/google/logger"
)

type PgCollectQueriesOptimization struct {
	logger        logging.Logger
	configuration *config.Config
}

func NewPgCollectQueriesOptimization(logger logging.Logger, configuration *config.Config) *PgCollectQueriesOptimization {
	return &PgCollectQueriesOptimization{
		logger:        logger,
		configuration: configuration,
	}
}

func (PgCollectQueriesOptimization *PgCollectQueriesOptimization) GetMetrics(metrics *models.Metrics) error {
	defer u.HandlePanic(PgCollectQueriesOptimization.configuration, PgCollectQueriesOptimization.logger)

	var dbname, query_id, query string
	var calls int
	var total_time, mean_time, rows_query float64
	output_digest := make(map[string]models.MetricGroupValue)

	// Check if pg_stat_statements extension is available
	var extension_exists bool
	err := models.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')").Scan(&extension_exists)
	if err != nil {
		PgCollectQueriesOptimization.logger.Error("Error checking pg_stat_statements extension: ", err)
		return err
	}

	if !extension_exists {
		PgCollectQueriesOptimization.logger.Info("pg_stat_statements extension is not installed, skipping query collection")
		metrics.DB.Queries = nil
		return nil
	}

	// Collect query statistics from pg_stat_statements
	rows, err := models.DB.Query(`
		SELECT 
			COALESCE(d.datname, NULL) as dbname,
			s.queryid::text as query_id,
			s.query as query_text,
			s.calls,
			s.total_exec_time as total_time,
			s.mean_exec_time as mean_time,
			s.rows as rows
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
				s.query as query_text,
				s.calls,
				s.total_time as total_time,
				s.mean_time as mean_time,
				s.rows as rows
			FROM pg_stat_statements s
			LEFT JOIN pg_database d ON d.oid = s.dbid
			WHERE s.calls > 0
			ORDER BY s.total_time DESC
			LIMIT 1000`)

		if err != nil {
			if err != sql.ErrNoRows {
				PgCollectQueriesOptimization.logger.Error(err)
			}
			metrics.DB.Queries = nil
			return nil
		}
	}
	defer rows.Close()

	for rows.Next() {
		err := rows.Scan(&dbname, &query_id, &query, &calls, &total_time, &mean_time, &rows_query)
		if err != nil {
			PgCollectQueriesOptimization.logger.Error(err)
			return err
		}

		// Convert to microseconds for compatibility with MySQL metrics
		total_time_us := int(total_time * 1000)
		mean_time_us := int(mean_time * 1000)
		query_text := ""
		output_digest[dbname+query_id] = models.MetricGroupValue{
			"schema_name":   dbname,
			"query_id":      query_id,
			"query":         query,
			"query_text":    query_text,
			"calls":         calls,
			"avg_time_us":   mean_time_us,
			"sum_time_us":   total_time_us,
			"rows_returned": rows_query,
		}
	}

	if len(output_digest) != 0 {
		for _, value := range output_digest {
			metrics.DB.Queries = append(metrics.DB.Queries, value)
		}
	} else {
		metrics.DB.Queries = nil
	}

	if !PgCollectQueriesOptimization.configuration.QueryOptimization {
		return nil
	}

	// List of databases
	var database string
	var output []string
	rows_database, err := models.DB.Query("SELECT datname FROM pg_database WHERE datistemplate = false ORDER BY datname")
	if err != nil {
		PgCollectQueriesOptimization.logger.Error(err)
		return err
	}
	defer rows_database.Close()

	for rows_database.Next() {
		err := rows_database.Scan(&database)
		if err != nil {
			PgCollectQueriesOptimization.logger.Error(err)
			return err
		}
		output = append(output, database)
	}
	metrics.DB.Metrics.Databases = output

	metrics.DB.QueriesOptimization = make(map[string][]models.MetricGroupValue)

	// Collect table information from information_schema
	type information_schema_table_type struct {
		TABLE_SCHEMA string
		TABLE_NAME   string
		TABLE_TYPE   string
	}
	var information_schema_table information_schema_table_type
	i := 0

	for _, database := range metrics.DB.Metrics.Databases {
		if u.IsSchemaNameExclude(database, PgCollectQueriesOptimization.configuration.DatabasesQueryOptimization) {
			continue
		}

		rows, err := models.DB.Query(`
			SELECT table_schema, table_name, table_type
			FROM information_schema.tables 
			WHERE table_catalog = $1 
			  AND table_schema NOT IN ('information_schema', 'pg_catalog')`, database)
		if err != nil {
			PgCollectQueriesOptimization.logger.Error(err)
		} else {
			defer rows.Close()
			for rows.Next() {
				err := rows.Scan(&information_schema_table.TABLE_SCHEMA, &information_schema_table.TABLE_NAME, &information_schema_table.TABLE_TYPE)
				if err != nil {
					PgCollectQueriesOptimization.logger.Error(err)
					return err
				}
				metrics.DB.QueriesOptimization["information_schema_tables"] = append(
					metrics.DB.QueriesOptimization["information_schema_tables"],
					models.MetricGroupValue{
						"TABLE_SCHEMA": information_schema_table.TABLE_SCHEMA,
						"TABLE_NAME":   information_schema_table.TABLE_NAME,
						"TABLE_TYPE":   information_schema_table.TABLE_TYPE,
					})
			}
		}
		i += 1
		if i%25 == 0 {
			time.Sleep(3 * time.Second)
		}
	}

	// Collect column information
	type information_schema_column_type struct {
		TABLE_SCHEMA     string
		TABLE_NAME       string
		COLUMN_NAME      string
		ORDINAL_POSITION string
		COLUMN_DEFAULT   string
		IS_NULLABLE      string
		DATA_TYPE        string
	}
	var information_schema_column information_schema_column_type

	rows, err = models.DB.Query(`
		SELECT table_schema, table_name, column_name, ordinal_position::text, 
		       COALESCE(column_default, ''), is_nullable, data_type
		FROM information_schema.columns 
		WHERE table_schema NOT IN ('information_schema', 'pg_catalog')
		ORDER BY table_schema, table_name, ordinal_position`)
	if err != nil {
		PgCollectQueriesOptimization.logger.Error(err)
	} else {
		defer rows.Close()
		for rows.Next() {
			err := rows.Scan(&information_schema_column.TABLE_SCHEMA, &information_schema_column.TABLE_NAME,
				&information_schema_column.COLUMN_NAME, &information_schema_column.ORDINAL_POSITION,
				&information_schema_column.COLUMN_DEFAULT, &information_schema_column.IS_NULLABLE,
				&information_schema_column.DATA_TYPE)
			if err != nil {
				PgCollectQueriesOptimization.logger.Error(err)
				return err
			}
			metrics.DB.QueriesOptimization["information_schema_columns"] = append(
				metrics.DB.QueriesOptimization["information_schema_columns"],
				models.MetricGroupValue{
					"TABLE_SCHEMA":     information_schema_column.TABLE_SCHEMA,
					"TABLE_NAME":       information_schema_column.TABLE_NAME,
					"COLUMN_NAME":      information_schema_column.COLUMN_NAME,
					"ORDINAL_POSITION": information_schema_column.ORDINAL_POSITION,
					"COLUMN_DEFAULT":   information_schema_column.COLUMN_DEFAULT,
					"IS_NULLABLE":      information_schema_column.IS_NULLABLE,
					"DATA_TYPE":        information_schema_column.DATA_TYPE,
				})
		}
	}

	PgCollectQueriesOptimization.logger.V(5).Info("collectMetrics ", metrics.DB.Queries)
	PgCollectQueriesOptimization.logger.V(5).Info("collectMetrics ", metrics.DB.QueriesOptimization)

	return nil
}

// PostgreSQL version of CollectionExplain - uses EXPLAIN (FORMAT JSON)
func PgCollectionExplain(digests map[string]models.MetricGroupValue, field_sorting string, logger logging.Logger, configuration *config.Config) {
	var explain, schema_name_conn, query_text string
	var i int
	var db *sql.DB

	pairs := make([][2]any, 0, len(digests))
	for k, v := range digests {
		pairs = append(pairs, [2]any{k, v[field_sorting]})
	}

	// Sort slice based on values
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i][1].(int) > pairs[j][1].(int)
	})

	for _, p := range pairs {
		k := p[0].(string)
		if i > 100 {
			break
		}

		if digests[k]["schema_name"].(string) != "postgres" &&
			digests[k]["schema_name"].(string) != "template0" &&
			digests[k]["schema_name"].(string) != "template1" &&
			(strings.Contains(digests[k]["query_text"].(string), "SELECT ") || strings.Contains(digests[k]["query_text"].(string), "select ")) &&
			digests[k]["explain"] == nil {

			if digests[k]["query_text"].(string) == "" {
				continue
			}
			if strings.Contains(digests[k]["query_text"].(string), "EXPLAIN (FORMAT JSON)") {
				continue
			}
			if u.IsSchemaNameExclude(digests[k]["schema_name"].(string), configuration.DatabasesQueryOptimization) {
				continue
			}

			if schema_name_conn != digests[k]["schema_name"].(string) {
				if db != nil {
					db.Close()
				}
				db = u.ConnectionDatabase(configuration, logger, digests[k]["schema_name"].(string))
				defer db.Close()
				schema_name_conn = digests[k]["schema_name"].(string)
			}

			// Try EXPLAIN for original query
			query_text = digests[k]["query_text"].(string)
			err := db.QueryRow("EXPLAIN (FORMAT JSON) " + query_text).Scan(&explain)
			if err != nil {
				logger.Error("Explain Error: ", err)
				digests[k]["explain_error"] = err.Error()
				logger.Error(query_text)
			} else {
				logger.V(5).Info(i, "OK")
				digests[k]["explain"] = explain
				i = i + 1
				continue
			}
		}
	}
}
