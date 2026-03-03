package postgresql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Releem/mysqlconfigurer/models"
	u "github.com/Releem/mysqlconfigurer/utils"
	"github.com/hashicorp/go-version"

	"github.com/Releem/mysqlconfigurer/config"
	logging "github.com/google/logger"
)

type DBCollectQueriesOptimization struct {
	logger        logging.Logger
	configuration *config.Config
}

func NewDBCollectQueriesOptimization(logger logging.Logger, configuration *config.Config) *DBCollectQueriesOptimization {
	return &DBCollectQueriesOptimization{
		logger:        logger,
		configuration: configuration,
	}
}

func (DBCollectQueriesOptimization *DBCollectQueriesOptimization) GetMetrics(metrics *models.Metrics) error {
	defer u.HandlePanic(DBCollectQueriesOptimization.configuration, DBCollectQueriesOptimization.logger)

	if !models.PgStatStatementsEnabled {
		DBCollectQueriesOptimization.logger.Info("pg_stat_statements extension is not installed, skipping query collection")
		metrics.DB.Queries = nil
		return nil
	}

	var queryid, datname, query string
	var calls int
	var total_exec_time, mean_exec_time float64
	output_digest := make(map[string]models.MetricGroupValue)

	ver_current, _ := version.NewVersion(metrics.DB.Info["Version"].(string))
	ver_postgresql, _ := version.NewVersion("13")
	ver_plan_cache_mode, _ := version.NewVersion("12")
	supportsParameterizedExplain := !ver_current.LessThan(ver_plan_cache_mode)
	// Collect DBMS internal metrics
	pgStatStatements := PG_STAT_STATEMENTS
	if ver_current.LessThan(ver_postgresql) {
		pgStatStatements = PG_STAT_STATEMENTS_OLD_VERSION
	}

	// Collect query statistics from pg_stat_statements
	rows, err := models.DB.Query(pgStatStatements)

	if err != nil {
		DBCollectQueriesOptimization.logger.Error(err)
	} else {
		defer rows.Close()
		for rows.Next() {
			err := rows.Scan(&datname, &queryid, &query, &calls, &total_exec_time, &mean_exec_time)
			if err != nil {
				DBCollectQueriesOptimization.logger.Error(err)
				return err
			}
			queryid = normalizePgQueryID(queryid)

			// Convert to microseconds for compatibility with MySQL metrics
			total_exec_time_us := total_exec_time * 1000
			mean_exec_time_us := mean_exec_time * 1000
			output_digest[datname+queryid] = models.MetricGroupValue{
				"datname":            datname,
				"queryid":            queryid,
				"query":              query,
				"query_text":         query,
				"calls":              calls,
				"total_exec_time_us": total_exec_time_us,
				"mean_exec_time_us":  mean_exec_time_us,
			}
		}
	}

	if DBCollectQueriesOptimization.configuration.QueryOptimization {
		CollectExplain(output_digest, "total_exec_time_us", supportsParameterizedExplain, DBCollectQueriesOptimization.logger, DBCollectQueriesOptimization.configuration)
		CollectExplain(output_digest, "mean_exec_time_us", supportsParameterizedExplain, DBCollectQueriesOptimization.logger, DBCollectQueriesOptimization.configuration)
	}
	for _, value := range output_digest {
		metrics.DB.Queries = append(metrics.DB.Queries, value)
	}

	if !DBCollectQueriesOptimization.configuration.QueryOptimization {
		return nil
	}

	metrics.DB.DatabaseSchema = make(map[string][]models.MetricGroupValue)
	i := 0
	for _, database := range metrics.DB.Metrics.Databases {
		if u.IsSchemaNameExclude(database, DBCollectQueriesOptimization.configuration.DatabasesQueryOptimization) {
			continue
		}
		CollectDbSchema(database, DBCollectQueriesOptimization.logger, metrics)

		i += 1
		if i%25 == 0 {
			time.Sleep(3 * time.Second)
		}
	}
	DBCollectQueriesOptimization.logger.V(5).Info("collectMetrics ", metrics.DB.Queries)
	DBCollectQueriesOptimization.logger.V(5).Info("collectMetrics ", metrics.DB.DatabaseSchema)

	return nil
}

func CollectDbSchema(database string, logger logging.Logger, metrics *models.Metrics) error {
	// Collect table information from information_schema
	type information_schema_table_type struct {
		TABLE_SCHEMA string
		TABLE_NAME   string
		TABLE_TYPE   string
	}
	var information_schema_table information_schema_table_type

	rows, err := models.DB.Query(`
		SELECT table_schema, table_name, table_type
		FROM information_schema.tables 
		WHERE table_catalog = $1
			AND table_schema NOT IN ('information_schema', 'pg_catalog')`, database)
	if err != nil {
		logger.Error(err)
	} else {
		defer rows.Close()
		for rows.Next() {
			err := rows.Scan(&information_schema_table.TABLE_SCHEMA, &information_schema_table.TABLE_NAME, &information_schema_table.TABLE_TYPE)
			if err != nil {
				logger.Error(err)
				return err
			}
			metrics.DB.DatabaseSchema["information_schema_tables"] = append(
				metrics.DB.DatabaseSchema["information_schema_tables"],
				models.MetricGroupValue{
					"TABLE_SCHEMA": information_schema_table.TABLE_SCHEMA,
					"TABLE_NAME":   information_schema_table.TABLE_NAME,
					"TABLE_TYPE":   information_schema_table.TABLE_TYPE,
				})
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
		WHERE table_catalog = $1
			AND table_schema NOT IN ('information_schema', 'pg_catalog')
		ORDER BY table_schema, table_name, ordinal_position`, database)
	if err != nil {
		logger.Error(err)
	} else {
		defer rows.Close()
		for rows.Next() {
			err := rows.Scan(&information_schema_column.TABLE_SCHEMA, &information_schema_column.TABLE_NAME,
				&information_schema_column.COLUMN_NAME, &information_schema_column.ORDINAL_POSITION,
				&information_schema_column.COLUMN_DEFAULT, &information_schema_column.IS_NULLABLE,
				&information_schema_column.DATA_TYPE)
			if err != nil {
				logger.Error(err)
				return err
			}
			metrics.DB.DatabaseSchema["information_schema_columns"] = append(
				metrics.DB.DatabaseSchema["information_schema_columns"],
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

	logger.V(5).Info("collectMetrics ", metrics.DB.DatabaseSchema)

	return nil
}

// PostgreSQL version of CollectionExplain - uses EXPLAIN (FORMAT JSON)
func CollectExplain(digests map[string]models.MetricGroupValue, field_sorting string, supportsParameterizedExplain bool, logger logging.Logger, configuration *config.Config) {
	var schema_name_conn string
	var i int
	var db *sql.DB

	pairs := make([][2]interface{}, 0, len(digests))
	for k, v := range digests {
		pairs = append(pairs, [2]interface{}{k, v[field_sorting]})
	}
	//logger.Println(pairs)
	// Sort slice based on values
	sort.Slice(pairs, func(i, j int) bool {
		return int(pairs[i][1].(float64)) > int(pairs[j][1].(float64))
	})

	for _, p := range pairs {
		k := p[0].(string)
		if i > 100 {
			break
		}

		if digests[k]["datname"].(string) == "postgres" ||
			digests[k]["datname"].(string) == "template0" ||
			digests[k]["datname"].(string) == "template1" ||
			digests[k]["datname"].(string) == "NULL" ||
			!(strings.Contains(digests[k]["query_text"].(string), "SELECT ") || strings.Contains(digests[k]["query_text"].(string), "select ")) ||
			digests[k]["explain"] != nil {
			continue
		}
		if digests[k]["query_text"].(string) == "" {
			continue
		}
		if strings.Contains(digests[k]["query_text"].(string), "EXPLAIN (FORMAT JSON)") {
			continue
		}
		if strings.Contains(digests[k]["query_text"].(string), "PREPARE") {
			continue
		}
		if u.IsSchemaNameExclude(digests[k]["datname"].(string), configuration.DatabasesQueryOptimization) {
			continue
		}

		if schema_name_conn != digests[k]["datname"].(string) {
			if db != nil {
				db.Close()
			}
			db = u.ConnectionDatabase(configuration, logger, digests[k]["datname"].(string))
			defer db.Close()
			schema_name_conn = digests[k]["datname"].(string)
		}
		query_explain, err := ExecuteExplain(db, digests[k]["queryid"].(string), digests[k]["query_text"].(string), supportsParameterizedExplain, logger)
		if err != nil {
			digests[k]["explain_error"] = err.Error()
		}
		if query_explain != "" {
			logger.Info(i, " OK")
			logger.Info("queryText: ", digests[k]["query_text"].(string))
			logger.Info("explain: ", query_explain)
			digests[k]["explain"] = query_explain
			i = i + 1
		}
	}
}

func ExecuteExplain(db *sql.DB, queryId string, queryText string, supportsParameterizedExplain bool, logger logging.Logger) (string, error) {
	var explain, query_text string
	var explain_error error
	explain_error = nil

	if supportsParameterizedExplain && containsUnquotedPgParameter(queryText) {
		explainPrepared, errPrepared := executePreparedExplain(db, queryId, queryText)
		if errPrepared != nil {
			logger.Error("Explain prepared statement error: ", errPrepared, "; queryText: ", queryText)
			if isExplainPermissionError(errPrepared) {
				return explainPrepared, errors.New("need_grant_permission")
			}
			explain_error = errPrepared
		} else {
			return explainPrepared, nil
		}
	}

	//Try exec EXPLAIN for origin query
	err := db.QueryRow("EXPLAIN (FORMAT JSON) " + queryText).Scan(&explain)
	if err != nil {
		logger.Error("Explain Error: ", err)
		if isExplainPermissionError(err) {
			explain_error = errors.New("need_grant_permission")
			return explain, explain_error
		} else {
			explain_error = err
		}
	} else {
		return explain, explain_error
	}

	//Try exec EXPLAIN for  query with replace "\"" on "'"
	query_text = strings.Replace(queryText, "\"", "'", -1)
	err_1 := db.QueryRow("EXPLAIN (FORMAT JSON) " + query_text).Scan(&explain)
	if err_1 != nil {
		logger.Error("Explain Error: ", err_1)
		if isExplainPermissionError(err_1) {
			explain_error = errors.New("need_grant_permission")
			return explain, explain_error
		} else {
			explain_error = err_1
		}
	} else {
		return explain, explain_error
	}

	//Try exec EXPLAIN for  query with replace "\"" on "`"
	query_text = strings.Replace(queryText, "\"", "`", -1)
	err_2 := db.QueryRow("EXPLAIN (FORMAT JSON) " + query_text).Scan(&explain)
	if err_2 != nil {
		logger.Error("Explain Error: ", err_2)
		if isExplainPermissionError(err_2) {
			explain_error = errors.New("need_grant_permission")
			return explain, explain_error
		} else {
			explain_error = err_2
		}
	} else {
		return explain, explain_error
	}

	return explain, explain_error
}

func isExplainPermissionError(err error) bool {
	if err == nil {
		return false
	}
	errText := strings.ToLower(err.Error())
	return strings.Contains(errText, "select command denied to user") ||
		strings.Contains(errText, "access denied for user") ||
		strings.Contains(errText, "permission denied")
}

func containsUnquotedPgParameter(query string) bool {
	inSingleQuote := false
	for i := 0; i < len(query); i++ {
		ch := query[i]
		if ch == '\'' {
			if inSingleQuote && i+1 < len(query) && query[i+1] == '\'' {
				i++
				continue
			}
			inSingleQuote = !inSingleQuote
			continue
		}
		if inSingleQuote || ch != '$' || i+1 >= len(query) {
			continue
		}
		if query[i+1] < '0' || query[i+1] > '9' {
			continue
		}
		return true
	}
	return false
}

func executePreparedExplain(db *sql.DB, queryId string, queryText string) (string, error) {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	stmtName := "releem_" + queryId

	if _, err = conn.ExecContext(ctx, "SET plan_cache_mode = force_generic_plan"); err != nil {
		return "", err
	}
	query := fmt.Sprintf("PREPARE %s AS %s", stmtName, queryText)
	if _, err = conn.ExecContext(ctx, query); err != nil {
		return "", err
	}
	defer conn.ExecContext(ctx, "DEALLOCATE PREPARE "+stmtName)

	var paramsCount int
	if err = conn.QueryRowContext(ctx, "SELECT COALESCE(cardinality(parameter_types), 0) FROM pg_prepared_statements WHERE name = $1", stmtName).Scan(&paramsCount); err != nil {
		return "", err
	}

	executeQuery := "EXPLAIN (FORMAT JSON) EXECUTE " + stmtName
	if paramsCount > 0 {
		nullParams := strings.TrimRight(strings.Repeat("NULL,", paramsCount), ",")
		executeQuery += "(" + nullParams + ")"
	}

	var explain string
	if err = conn.QueryRowContext(ctx, executeQuery).Scan(&explain); err != nil {
		return "", err
	}
	return explain, nil
}

func normalizePgQueryID(queryID string) string {
	trimmed := strings.TrimSpace(queryID)
	if trimmed == "" {
		return queryID
	}

	if signedValue, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return strconv.FormatUint(uint64(signedValue), 10)
	}

	if unsignedValue, err := strconv.ParseUint(trimmed, 10, 64); err == nil {
		return strconv.FormatUint(unsignedValue, 10)
	}

	return queryID
}
