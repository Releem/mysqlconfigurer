package metrics

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

type DbCollectQueriesOptimization struct {
	logger        logging.Logger
	configuration *config.Config
}

func NewDbCollectQueriesOptimization(logger logging.Logger, configuration *config.Config) *DbCollectQueriesOptimization {
	return &DbCollectQueriesOptimization{
		logger:        logger,
		configuration: configuration,
	}
}
func FilterQueryString(DatabasesQueryOptimization string, FieldName string) string {
	var DatabasesString string
	if DatabasesQueryOptimization != "" {
		DatabasesQueryOptimizationSlice := strings.Split(DatabasesQueryOptimization, `,`)
		for i, DbName := range DatabasesQueryOptimizationSlice {
			DatabasesString += "'" + DbName + "'"
			if i < (len(DatabasesQueryOptimizationSlice) - 1) {
				DatabasesString += ","
			}
		}
		return " WHERE " + FieldName + " IN (" + DatabasesString + ")"
	} else {
		return ""
	}
}
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
func (DbCollectQueriesOptimization *DbCollectQueriesOptimization) GetMetrics(metrics *models.Metrics) error {
	defer u.HandlePanic(DbCollectQueriesOptimization.configuration, DbCollectQueriesOptimization.logger)

	var schema_name, query, query_text, query_id string
	var SUM_LOCK_TIME, SUM_ERRORS, SUM_WARNINGS, SUM_ROWS_AFFECTED, SUM_ROWS_SENT, SUM_ROWS_EXAMINED, SUM_CREATED_TMP_DISK_TABLES, SUM_CREATED_TMP_TABLES, SUM_SELECT_FULL_JOIN, SUM_SELECT_FULL_RANGE_JOIN, SUM_SELECT_RANGE, SUM_SELECT_RANGE_CHECK, SUM_SELECT_SCAN, SUM_SORT_MERGE_PASSES, SUM_SORT_RANGE, SUM_SORT_ROWS, SUM_SORT_SCAN, SUM_NO_INDEX_USED, SUM_NO_GOOD_INDEX_USED uint64
	var calls, avg_time_us, sum_time_us int
	output_digest := make(map[string]models.MetricGroupValue)

	rows, err := models.DB.Query("SELECT IFNULL(schema_name, 'NULL') as schema_name, IFNULL(digest, 'NULL') as query_id, IFNULL(digest_text, 'NULL') as query, IFNULL(QUERY_SAMPLE_TEXT, 'NULL') as query_text, count_star as calls, round(avg_timer_wait/1000000, 0) as avg_time_us, round(SUM_TIMER_WAIT/1000000, 0) as sum_time_us, IFNULL(SUM_LOCK_TIME, 'NULL') as SUM_LOCK_TIME, IFNULL(SUM_ERRORS, 'NULL') as SUM_ERRORS, IFNULL(SUM_WARNINGS, 'NULL') as SUM_WARNINGS, IFNULL(SUM_ROWS_AFFECTED, 'NULL') as SUM_ROWS_AFFECTED, IFNULL(SUM_ROWS_SENT, 'NULL') as SUM_ROWS_SENT, IFNULL(SUM_ROWS_EXAMINED, 'NULL') as SUM_ROWS_EXAMINED, IFNULL(SUM_CREATED_TMP_DISK_TABLES, 'NULL') as SUM_CREATED_TMP_DISK_TABLES, IFNULL(SUM_CREATED_TMP_TABLES, 'NULL') as SUM_CREATED_TMP_TABLES, IFNULL(SUM_SELECT_FULL_JOIN, 'NULL') as SUM_SELECT_FULL_JOIN, IFNULL(SUM_SELECT_FULL_RANGE_JOIN, 'NULL') as SUM_SELECT_FULL_RANGE_JOIN, IFNULL(SUM_SELECT_RANGE, 'NULL') as SUM_SELECT_RANGE, IFNULL(SUM_SELECT_RANGE_CHECK, 'NULL') as SUM_SELECT_RANGE_CHECK, IFNULL(SUM_SELECT_SCAN, 'NULL') as SUM_SELECT_SCAN, IFNULL(SUM_SORT_MERGE_PASSES, 'NULL') as SUM_SORT_MERGE_PASSES, IFNULL(SUM_SORT_RANGE, 'NULL') as SUM_SORT_RANGE, IFNULL(SUM_SORT_ROWS, 'NULL') as SUM_SORT_ROWS, IFNULL(SUM_SORT_SCAN, 'NULL') as SUM_SORT_SCAN, IFNULL(SUM_NO_INDEX_USED, 'NULL') as SUM_NO_INDEX_USED, IFNULL(SUM_NO_GOOD_INDEX_USED, 'NULL') as SUM_NO_GOOD_INDEX_USED FROM performance_schema.events_statements_summary_by_digest WHERE `digest_text` NOT LIKE 'EXPLAIN %'")
	if err != nil {
		if err != sql.ErrNoRows && !strings.Contains(err.Error(), "Unknown column") {
			DbCollectQueriesOptimization.logger.Error(err)
		}
		rows, err = models.DB.Query("SELECT IFNULL(schema_name, 'NULL') as schema_name, IFNULL(digest, 'NULL') as query_id, IFNULL(digest_text, 'NULL') as query, count_star as calls, round(avg_timer_wait/1000000, 0) as avg_time_us, round(SUM_TIMER_WAIT/1000000, 0) as sum_time_us, IFNULL(SUM_LOCK_TIME, 'NULL') as SUM_LOCK_TIME, IFNULL(SUM_ERRORS, 'NULL') as SUM_ERRORS, IFNULL(SUM_WARNINGS, 'NULL') as SUM_WARNINGS, IFNULL(SUM_ROWS_AFFECTED, 'NULL') as SUM_ROWS_AFFECTED, IFNULL(SUM_ROWS_SENT, 'NULL') as SUM_ROWS_SENT, IFNULL(SUM_ROWS_EXAMINED, 'NULL') as SUM_ROWS_EXAMINED, IFNULL(SUM_CREATED_TMP_DISK_TABLES, 'NULL') as SUM_CREATED_TMP_DISK_TABLES, IFNULL(SUM_CREATED_TMP_TABLES, 'NULL') as SUM_CREATED_TMP_TABLES, IFNULL(SUM_SELECT_FULL_JOIN, 'NULL') as SUM_SELECT_FULL_JOIN, IFNULL(SUM_SELECT_FULL_RANGE_JOIN, 'NULL') as SUM_SELECT_FULL_RANGE_JOIN, IFNULL(SUM_SELECT_RANGE, 'NULL') as SUM_SELECT_RANGE, IFNULL(SUM_SELECT_RANGE_CHECK, 'NULL') as SUM_SELECT_RANGE_CHECK, IFNULL(SUM_SELECT_SCAN, 'NULL') as SUM_SELECT_SCAN, IFNULL(SUM_SORT_MERGE_PASSES, 'NULL') as SUM_SORT_MERGE_PASSES, IFNULL(SUM_SORT_RANGE, 'NULL') as SUM_SORT_RANGE, IFNULL(SUM_SORT_ROWS, 'NULL') as SUM_SORT_ROWS, IFNULL(SUM_SORT_SCAN, 'NULL') as SUM_SORT_SCAN, IFNULL(SUM_NO_INDEX_USED, 'NULL') as SUM_NO_INDEX_USED, IFNULL(SUM_NO_GOOD_INDEX_USED, 'NULL') as SUM_NO_GOOD_INDEX_USED FROM performance_schema.events_statements_summary_by_digest WHERE `digest_text` NOT LIKE 'EXPLAIN %'")
		if err != nil {
			if err != sql.ErrNoRows {
				DbCollectQueriesOptimization.logger.Error(err)
			}
		} else {
			for rows.Next() {
				err := rows.Scan(&schema_name, &query_id, &query, &calls, &avg_time_us, &sum_time_us, &SUM_LOCK_TIME, &SUM_ERRORS, &SUM_WARNINGS, &SUM_ROWS_AFFECTED, &SUM_ROWS_SENT, &SUM_ROWS_EXAMINED, &SUM_CREATED_TMP_DISK_TABLES, &SUM_CREATED_TMP_TABLES, &SUM_SELECT_FULL_JOIN, &SUM_SELECT_FULL_RANGE_JOIN, &SUM_SELECT_RANGE, &SUM_SELECT_RANGE_CHECK, &SUM_SELECT_SCAN, &SUM_SORT_MERGE_PASSES, &SUM_SORT_RANGE, &SUM_SORT_ROWS, &SUM_SORT_SCAN, &SUM_NO_INDEX_USED, &SUM_NO_GOOD_INDEX_USED)
				if err != nil {
					DbCollectQueriesOptimization.logger.Error(err)
					return err
				}
				models.SqlTextMutex.RLock()
				_, ok_schema_name := models.SqlText[schema_name]
				_, ok_query_id := models.SqlText[schema_name][query_id]

				if ok_schema_name && ok_query_id {
					query_text = models.SqlText[schema_name][query_id]
				} else {
					query_text = ""
				}
				models.SqlTextMutex.RUnlock()
				output_digest[schema_name+query_id] = models.MetricGroupValue{"schema_name": schema_name, "query_id": query_id, "query": query, "query_text": query_text, "calls": calls, "avg_time_us": avg_time_us, "sum_time_us": sum_time_us, "SUM_LOCK_TIME": SUM_LOCK_TIME, "SUM_ERRORS": SUM_ERRORS, "SUM_WARNINGS": SUM_WARNINGS, "SUM_ROWS_AFFECTED": SUM_ROWS_AFFECTED, "SUM_ROWS_SENT": SUM_ROWS_SENT, "SUM_ROWS_EXAMINED": SUM_ROWS_EXAMINED, "SUM_CREATED_TMP_DISK_TABLES": SUM_CREATED_TMP_DISK_TABLES, "SUM_CREATED_TMP_TABLES": SUM_CREATED_TMP_TABLES, "SUM_SELECT_FULL_JOIN": SUM_SELECT_FULL_JOIN, "SUM_SELECT_FULL_RANGE_JOIN": SUM_SELECT_FULL_RANGE_JOIN, "SUM_SELECT_RANGE": SUM_SELECT_RANGE, "SUM_SELECT_RANGE_CHECK": SUM_SELECT_RANGE_CHECK, "SUM_SELECT_SCAN": SUM_SELECT_SCAN, "SUM_SORT_MERGE_PASSES": SUM_SORT_MERGE_PASSES, "SUM_SORT_RANGE": SUM_SORT_RANGE, "SUM_SORT_ROWS": SUM_SORT_ROWS, "SUM_SORT_SCAN": SUM_SORT_SCAN, "SUM_NO_INDEX_USED": SUM_NO_INDEX_USED, "SUM_NO_GOOD_INDEX_USED": SUM_NO_GOOD_INDEX_USED}
			}
			rows.Close()

			if DbCollectQueriesOptimization.configuration.QueryOptimization {
				CollectionExplain(output_digest, "sum_time_us", DbCollectQueriesOptimization.logger, DbCollectQueriesOptimization.configuration, true)
				CollectionExplain(output_digest, "avg_time_us", DbCollectQueriesOptimization.logger, DbCollectQueriesOptimization.configuration, true)
			}
		}

	} else {
		for rows.Next() {
			err := rows.Scan(&schema_name, &query_id, &query, &query_text, &calls, &avg_time_us, &sum_time_us, &SUM_LOCK_TIME, &SUM_ERRORS, &SUM_WARNINGS, &SUM_ROWS_AFFECTED, &SUM_ROWS_SENT, &SUM_ROWS_EXAMINED, &SUM_CREATED_TMP_DISK_TABLES, &SUM_CREATED_TMP_TABLES, &SUM_SELECT_FULL_JOIN, &SUM_SELECT_FULL_RANGE_JOIN, &SUM_SELECT_RANGE, &SUM_SELECT_RANGE_CHECK, &SUM_SELECT_SCAN, &SUM_SORT_MERGE_PASSES, &SUM_SORT_RANGE, &SUM_SORT_ROWS, &SUM_SORT_SCAN, &SUM_NO_INDEX_USED, &SUM_NO_GOOD_INDEX_USED)
			if err != nil {
				DbCollectQueriesOptimization.logger.Error(err)
				return err
			}
			output_digest[schema_name+query_id] = models.MetricGroupValue{"schema_name": schema_name, "query_id": query_id, "query": query, "query_text": query_text, "calls": calls, "avg_time_us": avg_time_us, "sum_time_us": sum_time_us, "SUM_LOCK_TIME": SUM_LOCK_TIME, "SUM_ERRORS": SUM_ERRORS, "SUM_WARNINGS": SUM_WARNINGS, "SUM_ROWS_AFFECTED": SUM_ROWS_AFFECTED, "SUM_ROWS_SENT": SUM_ROWS_SENT, "SUM_ROWS_EXAMINED": SUM_ROWS_EXAMINED, "SUM_CREATED_TMP_DISK_TABLES": SUM_CREATED_TMP_DISK_TABLES, "SUM_CREATED_TMP_TABLES": SUM_CREATED_TMP_TABLES, "SUM_SELECT_FULL_JOIN": SUM_SELECT_FULL_JOIN, "SUM_SELECT_FULL_RANGE_JOIN": SUM_SELECT_FULL_RANGE_JOIN, "SUM_SELECT_RANGE": SUM_SELECT_RANGE, "SUM_SELECT_RANGE_CHECK": SUM_SELECT_RANGE_CHECK, "SUM_SELECT_SCAN": SUM_SELECT_SCAN, "SUM_SORT_MERGE_PASSES": SUM_SORT_MERGE_PASSES, "SUM_SORT_RANGE": SUM_SORT_RANGE, "SUM_SORT_ROWS": SUM_SORT_ROWS, "SUM_SORT_SCAN": SUM_SORT_SCAN, "SUM_NO_INDEX_USED": SUM_NO_INDEX_USED, "SUM_NO_GOOD_INDEX_USED": SUM_NO_GOOD_INDEX_USED}
		}
		rows.Close()

		if DbCollectQueriesOptimization.configuration.QueryOptimization {
			CollectionExplain(output_digest, "sum_time_us", DbCollectQueriesOptimization.logger, DbCollectQueriesOptimization.configuration, false)
			CollectionExplain(output_digest, "avg_time_us", DbCollectQueriesOptimization.logger, DbCollectQueriesOptimization.configuration, false)
		}
	}
	if len(output_digest) != 0 {
		for _, value := range output_digest {
			metrics.DB.Queries = append(metrics.DB.Queries, value)
		}
	} else {
		metrics.DB.Queries = nil
	}

	if !DbCollectQueriesOptimization.configuration.QueryOptimization {
		return nil
	}

	//list of databases
	var database string
	var output []string
	rows_database, err := models.DB.Query("SELECT table_schema FROM INFORMATION_SCHEMA.tables group BY table_schema")
	if err != nil {
		DbCollectQueriesOptimization.logger.Error(err)
		return err
	}
	for rows_database.Next() {
		err := rows_database.Scan(&database)
		if err != nil {
			DbCollectQueriesOptimization.logger.Error(err)
			return err
		}
		output = append(output, database)
	}
	rows_database.Close()
	metrics.DB.Metrics.Databases = output

	metrics.DB.QueriesOptimization = make(map[string][]models.MetricGroupValue)
	type information_schema_table_type struct {
		TABLE_SCHEMA    string
		TABLE_NAME      string
		TABLE_TYPE      string
		ENGINE          string
		ROW_FORMAT      string
		TABLE_ROWS      string
		AVG_ROW_LENGTH  string
		MAX_DATA_LENGTH string
		DATA_LENGTH     string
		INDEX_LENGTH    string
		TABLE_COLLATION string
		DATA_FREE       string
	}
	var information_schema_table information_schema_table_type
	i := 0
	for _, database := range metrics.DB.Metrics.Databases {
		if IsSchemaNameExclude(database, DbCollectQueriesOptimization.configuration.DatabasesQueryOptimization) {
			continue
		}
		rows, err := models.DB.Query(`SELECT IFNULL(TABLE_SCHEMA, 'NULL') as TABLE_SCHEMA, IFNULL(TABLE_NAME, 'NULL') as TABLE_NAME, IFNULL(TABLE_TYPE, 'NULL') as TABLE_TYPE,  IFNULL(ENGINE, 'NULL') as ENGINE, IFNULL(ROW_FORMAT, 'NULL') as ROW_FORMAT, IFNULL(TABLE_ROWS, 'NULL') as TABLE_ROWS, IFNULL(AVG_ROW_LENGTH, 'NULL') as AVG_ROW_LENGTH, IFNULL(MAX_DATA_LENGTH, 'NULL') as MAX_DATA_LENGTH, IFNULL(DATA_LENGTH, 'NULL') as DATA_LENGTH, IFNULL(INDEX_LENGTH, 'NULL') as INDEX_LENGTH, IFNULL(TABLE_COLLATION, 'NULL') as TABLE_COLLATION, IFNULL(DATA_FREE, 'NULL') as DATA_FREE FROM information_schema.tables WHERE TABLE_SCHEMA = ? `, database)
		if err != nil {
			DbCollectQueriesOptimization.logger.Error(err)
		} else {
			for rows.Next() {
				err := rows.Scan(&information_schema_table.TABLE_SCHEMA, &information_schema_table.TABLE_NAME, &information_schema_table.TABLE_TYPE, &information_schema_table.ENGINE, &information_schema_table.ROW_FORMAT, &information_schema_table.TABLE_ROWS, &information_schema_table.AVG_ROW_LENGTH, &information_schema_table.MAX_DATA_LENGTH, &information_schema_table.DATA_LENGTH, &information_schema_table.INDEX_LENGTH, &information_schema_table.TABLE_COLLATION, &information_schema_table.DATA_FREE)
				if err != nil {
					DbCollectQueriesOptimization.logger.Error(err)
					return err
				}
				metrics.DB.QueriesOptimization["information_schema_tables"] = append(metrics.DB.QueriesOptimization["information_schema_tables"], models.MetricGroupValue{"TABLE_SCHEMA": information_schema_table.TABLE_SCHEMA, "TABLE_NAME": information_schema_table.TABLE_NAME, "TABLE_TYPE": information_schema_table.TABLE_TYPE, "ENGINE": information_schema_table.ENGINE, "ROW_FORMAT": information_schema_table.ROW_FORMAT, "TABLE_ROWS": information_schema_table.TABLE_ROWS, "AVG_ROW_LENGTH": information_schema_table.AVG_ROW_LENGTH, "MAX_DATA_LENGTH": information_schema_table.MAX_DATA_LENGTH, "DATA_LENGTH": information_schema_table.DATA_LENGTH, "INDEX_LENGTH": information_schema_table.INDEX_LENGTH, "TABLE_COLLATION": information_schema_table.TABLE_COLLATION, "DATA_FREE": information_schema_table.DATA_FREE})
			}
		}
		rows.Close()
		i += 1
		if i%25 == 0 {
			time.Sleep(3 * time.Second)
		}
	}

	type information_schema_column_type struct {
		TABLE_SCHEMA             string
		TABLE_NAME               string
		COLUMN_NAME              string
		ORDINAL_POSITION         string
		COLUMN_DEFAULT           string
		IS_NULLABLE              string
		DATA_TYPE                string
		CHARACTER_MAXIMUM_LENGTH string
		NUMERIC_PRECISION        string
		NUMERIC_SCALE            string
		CHARACTER_SET_NAME       string
		COLLATION_NAME           string
		COLUMN_TYPE              string
		COLUMN_KEY               string
		EXTRA                    string
	}
	var information_schema_column information_schema_column_type

	rows, err = models.DB.Query("SELECT IFNULL(TABLE_SCHEMA, 'NULL') as TABLE_SCHEMA, IFNULL(TABLE_NAME, 'NULL') as TABLE_NAME, IFNULL(COLUMN_NAME, 'NULL') as COLUMN_NAME, IFNULL(ORDINAL_POSITION, 'NULL') as ORDINAL_POSITION, IFNULL(COLUMN_DEFAULT, 'NULL') as COLUMN_DEFAULT, IFNULL(IS_NULLABLE, 'NULL') as IS_NULLABLE, IFNULL(DATA_TYPE, 'NULL') as DATA_TYPE, IFNULL(CHARACTER_MAXIMUM_LENGTH, 'NULL') as CHARACTER_MAXIMUM_LENGTH, IFNULL(NUMERIC_PRECISION, 'NULL') as NUMERIC_PRECISION, IFNULL(NUMERIC_SCALE, 'NULL') as NUMERIC_SCALE, IFNULL(CHARACTER_SET_NAME, 'NULL') as CHARACTER_SET_NAME, IFNULL(COLLATION_NAME, 'NULL') as COLLATION_NAME, IFNULL(COLUMN_TYPE, 'NULL') as COLUMN_TYPE, IFNULL(COLUMN_KEY, 'NULL') as COLUMN_KEY, IFNULL(EXTRA, 'NULL') as EXTRA FROM information_schema.columns" + FilterQueryString(DbCollectQueriesOptimization.configuration.DatabasesQueryOptimization, "TABLE_SCHEMA"))
	if err != nil {
		DbCollectQueriesOptimization.logger.Error(err)
	} else {
		for rows.Next() {
			err := rows.Scan(&information_schema_column.TABLE_SCHEMA, &information_schema_column.TABLE_NAME, &information_schema_column.COLUMN_NAME, &information_schema_column.ORDINAL_POSITION, &information_schema_column.COLUMN_DEFAULT, &information_schema_column.IS_NULLABLE, &information_schema_column.DATA_TYPE, &information_schema_column.CHARACTER_MAXIMUM_LENGTH, &information_schema_column.NUMERIC_PRECISION, &information_schema_column.NUMERIC_SCALE, &information_schema_column.CHARACTER_SET_NAME, &information_schema_column.COLLATION_NAME, &information_schema_column.COLUMN_TYPE, &information_schema_column.COLUMN_KEY, &information_schema_column.EXTRA)
			if err != nil {
				DbCollectQueriesOptimization.logger.Error(err)
				return err
			}
			metrics.DB.QueriesOptimization["information_schema_columns"] = append(metrics.DB.QueriesOptimization["information_schema_columns"], models.MetricGroupValue{"TABLE_SCHEMA": information_schema_column.TABLE_SCHEMA, "TABLE_NAME": information_schema_column.TABLE_NAME, "COLUMN_NAME": information_schema_column.COLUMN_NAME, "ORDINAL_POSITION": information_schema_column.ORDINAL_POSITION, "COLUMN_DEFAULT": information_schema_column.COLUMN_DEFAULT, "IS_NULLABLE": information_schema_column.IS_NULLABLE, "DATA_TYPE": information_schema_column.DATA_TYPE, "CHARACTER_MAXIMUM_LENGTH": information_schema_column.CHARACTER_MAXIMUM_LENGTH, "NUMERIC_PRECISION": information_schema_column.NUMERIC_PRECISION, "NUMERIC_SCALE": information_schema_column.NUMERIC_SCALE, "CHARACTER_SET_NAME": information_schema_column.CHARACTER_SET_NAME, "COLLATION_NAME": information_schema_column.COLLATION_NAME, "COLUMN_TYPE": information_schema_column.COLUMN_TYPE, "COLUMN_KEY": information_schema_column.COLUMN_KEY, "EXTRA": information_schema_column.EXTRA})
		}
		rows.Close()
	}

	type information_schema_index_type struct {
		TABLE_SCHEMA string
		TABLE_NAME   string
		INDEX_NAME   string
		NON_UNIQUE   string
		SEQ_IN_INDEX string
		COLUMN_NAME  string
		COLLATION    string
		CARDINALITY  string
		SUB_PART     string
		PACKED       string
		NULLABLE     string
		INDEX_TYPE   string
		EXPRESSION   string
	}
	var information_schema_index information_schema_index_type

	rows, err = models.DB.Query("SELECT IFNULL(TABLE_SCHEMA, 'NULL') as TABLE_SCHEMA, IFNULL(TABLE_NAME, 'NULL') as TABLE_NAME, IFNULL(INDEX_NAME, 'NULL') as INDEX_NAME, IFNULL(NON_UNIQUE, 'NULL') as NON_UNIQUE, IFNULL(SEQ_IN_INDEX, 'NULL') as SEQ_IN_INDEX, IFNULL(COLUMN_NAME, 'NULL') as COLUMN_NAME, IFNULL(COLLATION, 'NULL') as COLLATION, IFNULL(CARDINALITY, 'NULL') as CARDINALITY, IFNULL(SUB_PART, 'NULL') as SUB_PART, IFNULL(PACKED, 'NULL') as PACKED, IFNULL(NULLABLE, 'NULL') as NULLABLE, IFNULL(INDEX_TYPE, 'NULL') as INDEX_TYPE, IFNULL(EXPRESSION, 'NULL') as EXPRESSION FROM information_schema.statistics" + FilterQueryString(DbCollectQueriesOptimization.configuration.DatabasesQueryOptimization, "TABLE_SCHEMA"))
	if err != nil {
		if err != sql.ErrNoRows && !strings.Contains(err.Error(), "Unknown column") {
			DbCollectQueriesOptimization.logger.Error(err)
		}
		rows, err = models.DB.Query("SELECT IFNULL(TABLE_SCHEMA, 'NULL') as TABLE_SCHEMA, IFNULL(TABLE_NAME, 'NULL') as TABLE_NAME, IFNULL(INDEX_NAME, 'NULL') as INDEX_NAME, IFNULL(NON_UNIQUE, 'NULL') as NON_UNIQUE, IFNULL(SEQ_IN_INDEX, 'NULL') as SEQ_IN_INDEX, IFNULL(COLUMN_NAME, 'NULL') as COLUMN_NAME, IFNULL(COLLATION, 'NULL') as COLLATION, IFNULL(CARDINALITY, 'NULL') as CARDINALITY, IFNULL(SUB_PART, 'NULL') as SUB_PART, IFNULL(PACKED, 'NULL') as PACKED, IFNULL(NULLABLE, 'NULL') as NULLABLE, IFNULL(INDEX_TYPE, 'NULL') as INDEX_TYPE FROM information_schema.statistics" + FilterQueryString(DbCollectQueriesOptimization.configuration.DatabasesQueryOptimization, "TABLE_SCHEMA"))
		if err != nil {
			DbCollectQueriesOptimization.logger.Error(err)
		} else {
			for rows.Next() {
				err := rows.Scan(&information_schema_index.TABLE_SCHEMA, &information_schema_index.TABLE_NAME, &information_schema_index.INDEX_NAME, &information_schema_index.NON_UNIQUE, &information_schema_index.SEQ_IN_INDEX, &information_schema_index.COLUMN_NAME, &information_schema_index.COLLATION, &information_schema_index.CARDINALITY, &information_schema_index.SUB_PART, &information_schema_index.PACKED, &information_schema_index.NULLABLE, &information_schema_index.INDEX_TYPE)
				if err != nil {
					DbCollectQueriesOptimization.logger.Error(err)
					return err
				}
				metrics.DB.QueriesOptimization["information_schema_indexes"] = append(metrics.DB.QueriesOptimization["information_schema_indexes"], models.MetricGroupValue{"TABLE_SCHEMA": information_schema_index.TABLE_SCHEMA, "TABLE_NAME": information_schema_index.TABLE_NAME, "INDEX_NAME": information_schema_index.INDEX_NAME, "NON_UNIQUE": information_schema_index.NON_UNIQUE, "SEQ_IN_INDEX": information_schema_index.SEQ_IN_INDEX, "COLUMN_NAME": information_schema_index.COLUMN_NAME, "COLLATION": information_schema_index.COLLATION, "CARDINALITY": information_schema_index.CARDINALITY, "SUB_PART": information_schema_index.SUB_PART, "PACKED": information_schema_index.PACKED, "NULLABLE": information_schema_index.NULLABLE, "INDEX_TYPE": information_schema_index.INDEX_TYPE})
			}
			rows.Close()
		}
	} else {
		for rows.Next() {
			err := rows.Scan(&information_schema_index.TABLE_SCHEMA, &information_schema_index.TABLE_NAME, &information_schema_index.INDEX_NAME, &information_schema_index.NON_UNIQUE, &information_schema_index.SEQ_IN_INDEX, &information_schema_index.COLUMN_NAME, &information_schema_index.COLLATION, &information_schema_index.CARDINALITY, &information_schema_index.SUB_PART, &information_schema_index.PACKED, &information_schema_index.NULLABLE, &information_schema_index.INDEX_TYPE, &information_schema_index.EXPRESSION)
			if err != nil {
				DbCollectQueriesOptimization.logger.Error(err)
				return err
			}
			metrics.DB.QueriesOptimization["information_schema_indexes"] = append(metrics.DB.QueriesOptimization["information_schema_indexes"], models.MetricGroupValue{"TABLE_SCHEMA": information_schema_index.TABLE_SCHEMA, "TABLE_NAME": information_schema_index.TABLE_NAME, "INDEX_NAME": information_schema_index.INDEX_NAME, "NON_UNIQUE": information_schema_index.NON_UNIQUE, "SEQ_IN_INDEX": information_schema_index.SEQ_IN_INDEX, "COLUMN_NAME": information_schema_index.COLUMN_NAME, "COLLATION": information_schema_index.COLLATION, "CARDINALITY": information_schema_index.CARDINALITY, "SUB_PART": information_schema_index.SUB_PART, "PACKED": information_schema_index.PACKED, "NULLABLE": information_schema_index.NULLABLE, "INDEX_TYPE": information_schema_index.INDEX_TYPE, "EXPRESSION": information_schema_index.EXPRESSION})
		}
		rows.Close()
	}

	type performance_schema_table_io_waits_summary_by_index_usage_type struct {
		OBJECT_TYPE      string
		OBJECT_SCHEMA    string
		OBJECT_NAME      string
		INDEX_NAME       string
		COUNT_STAR       string
		SUM_TIMER_WAIT   string
		MIN_TIMER_WAIT   string
		AVG_TIMER_WAIT   string
		MAX_TIMER_WAIT   string
		COUNT_READ       string
		SUM_TIMER_READ   string
		MIN_TIMER_READ   string
		AVG_TIMER_READ   string
		MAX_TIMER_READ   string
		COUNT_WRITE      string
		SUM_TIMER_WRITE  string
		MIN_TIMER_WRITE  string
		AVG_TIMER_WRITE  string
		MAX_TIMER_WRITE  string
		COUNT_FETCH      string
		SUM_TIMER_FETCH  string
		MIN_TIMER_FETCH  string
		AVG_TIMER_FETCH  string
		MAX_TIMER_FETCH  string
		COUNT_INSERT     string
		SUM_TIMER_INSERT string
		MIN_TIMER_INSERT string
		AVG_TIMER_INSERT string
		MAX_TIMER_INSERT string
		COUNT_UPDATE     string
		SUM_TIMER_UPDATE string
		MIN_TIMER_UPDATE string
		AVG_TIMER_UPDATE string
		MAX_TIMER_UPDATE string
		COUNT_DELETE     string
		SUM_TIMER_DELETE string
		MIN_TIMER_DELETE string
		AVG_TIMER_DELETE string
		MAX_TIMER_DELETE string
	}
	var performance_schema_table_io_waits_summary_by_index_usage performance_schema_table_io_waits_summary_by_index_usage_type

	rows, err = models.DB.Query("SELECT IFNULL(OBJECT_TYPE, 'NULL') as OBJECT_TYPE, IFNULL(OBJECT_SCHEMA, 'NULL') as  OBJECT_SCHEMA, IFNULL(OBJECT_NAME, 'NULL') as  OBJECT_NAME, IFNULL(INDEX_NAME, 'NULL') as  INDEX_NAME, IFNULL(COUNT_STAR, 'NULL') as  COUNT_STAR, IFNULL(SUM_TIMER_WAIT, 'NULL') as  SUM_TIMER_WAIT, IFNULL(MIN_TIMER_WAIT, 'NULL') as  MIN_TIMER_WAIT, IFNULL(AVG_TIMER_WAIT, 'NULL') as  AVG_TIMER_WAIT, IFNULL(MAX_TIMER_WAIT, 'NULL') as  MAX_TIMER_WAIT, IFNULL(COUNT_READ, 'NULL') as  COUNT_READ, IFNULL(SUM_TIMER_READ, 'NULL') as  SUM_TIMER_READ, IFNULL(MIN_TIMER_READ, 'NULL') as  MIN_TIMER_READ, IFNULL(AVG_TIMER_READ, 'NULL') as  AVG_TIMER_READ, IFNULL(MAX_TIMER_READ, 'NULL') as  MAX_TIMER_READ, IFNULL(COUNT_WRITE, 'NULL') as  COUNT_WRITE, IFNULL(SUM_TIMER_WRITE, 'NULL') as  SUM_TIMER_WRITE, IFNULL(MIN_TIMER_WRITE, 'NULL') as  MIN_TIMER_WRITE, IFNULL(AVG_TIMER_WRITE, 'NULL') as  AVG_TIMER_WRITE, IFNULL(MAX_TIMER_WRITE, 'NULL') as  MAX_TIMER_WRITE, IFNULL(COUNT_FETCH, 'NULL') as  COUNT_FETCH, IFNULL(SUM_TIMER_FETCH, 'NULL') as  SUM_TIMER_FETCH, IFNULL(MIN_TIMER_FETCH, 'NULL') as  MIN_TIMER_FETCH, IFNULL(AVG_TIMER_FETCH, 'NULL') as  AVG_TIMER_FETCH, IFNULL(MAX_TIMER_FETCH, 'NULL') as  MAX_TIMER_FETCH, IFNULL(COUNT_INSERT, 'NULL') as  COUNT_INSERT, IFNULL(SUM_TIMER_INSERT, 'NULL') as  SUM_TIMER_INSERT, IFNULL(MIN_TIMER_INSERT, 'NULL') as  MIN_TIMER_INSERT, IFNULL(AVG_TIMER_INSERT, 'NULL') as  AVG_TIMER_INSERT, IFNULL(MAX_TIMER_INSERT, 'NULL') as  MAX_TIMER_INSERT, IFNULL(COUNT_UPDATE, 'NULL') as  COUNT_UPDATE, IFNULL(SUM_TIMER_UPDATE, 'NULL') as  SUM_TIMER_UPDATE, IFNULL(MIN_TIMER_UPDATE, 'NULL') as  MIN_TIMER_UPDATE, IFNULL(AVG_TIMER_UPDATE, 'NULL') as  AVG_TIMER_UPDATE, IFNULL(MAX_TIMER_UPDATE, 'NULL') as  MAX_TIMER_UPDATE, IFNULL(COUNT_DELETE, 'NULL') as  COUNT_DELETE, IFNULL(SUM_TIMER_DELETE, 'NULL') as  SUM_TIMER_DELETE, IFNULL(MIN_TIMER_DELETE, 'NULL') as  MIN_TIMER_DELETE, IFNULL(AVG_TIMER_DELETE, 'NULL') as  AVG_TIMER_DELETE, IFNULL(MAX_TIMER_DELETE, 'NULL') as  MAX_TIMER_DELETE FROM performance_schema.table_io_waits_summary_by_index_usage" + FilterQueryString(DbCollectQueriesOptimization.configuration.DatabasesQueryOptimization, "OBJECT_SCHEMA"))
	if err != nil {
		DbCollectQueriesOptimization.logger.Error(err)
	} else {
		for rows.Next() {
			err := rows.Scan(&performance_schema_table_io_waits_summary_by_index_usage.OBJECT_TYPE, &performance_schema_table_io_waits_summary_by_index_usage.OBJECT_SCHEMA, &performance_schema_table_io_waits_summary_by_index_usage.OBJECT_NAME, &performance_schema_table_io_waits_summary_by_index_usage.INDEX_NAME, &performance_schema_table_io_waits_summary_by_index_usage.COUNT_STAR, &performance_schema_table_io_waits_summary_by_index_usage.SUM_TIMER_WAIT, &performance_schema_table_io_waits_summary_by_index_usage.MIN_TIMER_WAIT, &performance_schema_table_io_waits_summary_by_index_usage.AVG_TIMER_WAIT, &performance_schema_table_io_waits_summary_by_index_usage.MAX_TIMER_WAIT, &performance_schema_table_io_waits_summary_by_index_usage.COUNT_READ, &performance_schema_table_io_waits_summary_by_index_usage.SUM_TIMER_READ, &performance_schema_table_io_waits_summary_by_index_usage.MIN_TIMER_READ, &performance_schema_table_io_waits_summary_by_index_usage.AVG_TIMER_READ, &performance_schema_table_io_waits_summary_by_index_usage.MAX_TIMER_READ, &performance_schema_table_io_waits_summary_by_index_usage.COUNT_WRITE, &performance_schema_table_io_waits_summary_by_index_usage.SUM_TIMER_WRITE, &performance_schema_table_io_waits_summary_by_index_usage.MIN_TIMER_WRITE, &performance_schema_table_io_waits_summary_by_index_usage.AVG_TIMER_WRITE, &performance_schema_table_io_waits_summary_by_index_usage.MAX_TIMER_WRITE, &performance_schema_table_io_waits_summary_by_index_usage.COUNT_FETCH, &performance_schema_table_io_waits_summary_by_index_usage.SUM_TIMER_FETCH, &performance_schema_table_io_waits_summary_by_index_usage.MIN_TIMER_FETCH, &performance_schema_table_io_waits_summary_by_index_usage.AVG_TIMER_FETCH, &performance_schema_table_io_waits_summary_by_index_usage.MAX_TIMER_FETCH, &performance_schema_table_io_waits_summary_by_index_usage.COUNT_INSERT, &performance_schema_table_io_waits_summary_by_index_usage.SUM_TIMER_INSERT, &performance_schema_table_io_waits_summary_by_index_usage.MIN_TIMER_INSERT, &performance_schema_table_io_waits_summary_by_index_usage.AVG_TIMER_INSERT, &performance_schema_table_io_waits_summary_by_index_usage.MAX_TIMER_INSERT, &performance_schema_table_io_waits_summary_by_index_usage.COUNT_UPDATE, &performance_schema_table_io_waits_summary_by_index_usage.SUM_TIMER_UPDATE, &performance_schema_table_io_waits_summary_by_index_usage.MIN_TIMER_UPDATE, &performance_schema_table_io_waits_summary_by_index_usage.AVG_TIMER_UPDATE, &performance_schema_table_io_waits_summary_by_index_usage.MAX_TIMER_UPDATE, &performance_schema_table_io_waits_summary_by_index_usage.COUNT_DELETE, &performance_schema_table_io_waits_summary_by_index_usage.SUM_TIMER_DELETE, &performance_schema_table_io_waits_summary_by_index_usage.MIN_TIMER_DELETE, &performance_schema_table_io_waits_summary_by_index_usage.AVG_TIMER_DELETE, &performance_schema_table_io_waits_summary_by_index_usage.MAX_TIMER_DELETE)
			if err != nil {
				DbCollectQueriesOptimization.logger.Error(err)
				return err
			}
			metrics.DB.QueriesOptimization["performance_schema_table_io_waits_summary_by_index_usage"] = append(metrics.DB.QueriesOptimization["performance_schema_table_io_waits_summary_by_index_usage"], models.MetricGroupValue{"OBJECT_TYPE": performance_schema_table_io_waits_summary_by_index_usage.OBJECT_TYPE, "OBJECT_SCHEMA": performance_schema_table_io_waits_summary_by_index_usage.OBJECT_SCHEMA, "OBJECT_NAME": performance_schema_table_io_waits_summary_by_index_usage.OBJECT_NAME, "INDEX_NAME": performance_schema_table_io_waits_summary_by_index_usage.INDEX_NAME, "COUNT_STAR": performance_schema_table_io_waits_summary_by_index_usage.COUNT_STAR, "SUM_TIMER_WAIT": performance_schema_table_io_waits_summary_by_index_usage.SUM_TIMER_WAIT, "MIN_TIMER_WAIT": performance_schema_table_io_waits_summary_by_index_usage.MIN_TIMER_WAIT, "AVG_TIMER_WAIT": performance_schema_table_io_waits_summary_by_index_usage.AVG_TIMER_WAIT, "MAX_TIMER_WAIT": performance_schema_table_io_waits_summary_by_index_usage.MAX_TIMER_WAIT, "COUNT_READ": performance_schema_table_io_waits_summary_by_index_usage.COUNT_READ, "SUM_TIMER_READ": performance_schema_table_io_waits_summary_by_index_usage.SUM_TIMER_READ, "MIN_TIMER_READ": performance_schema_table_io_waits_summary_by_index_usage.MIN_TIMER_READ, "AVG_TIMER_READ": performance_schema_table_io_waits_summary_by_index_usage.AVG_TIMER_READ, "MAX_TIMER_READ": performance_schema_table_io_waits_summary_by_index_usage.MAX_TIMER_READ, "COUNT_WRITE": performance_schema_table_io_waits_summary_by_index_usage.COUNT_WRITE, "SUM_TIMER_WRITE": performance_schema_table_io_waits_summary_by_index_usage.SUM_TIMER_WRITE, "MIN_TIMER_WRITE": performance_schema_table_io_waits_summary_by_index_usage.MIN_TIMER_WRITE, "AVG_TIMER_WRITE": performance_schema_table_io_waits_summary_by_index_usage.AVG_TIMER_WRITE, "MAX_TIMER_WRITE": performance_schema_table_io_waits_summary_by_index_usage.MAX_TIMER_WRITE, "COUNT_FETCH": performance_schema_table_io_waits_summary_by_index_usage.COUNT_FETCH, "SUM_TIMER_FETCH": performance_schema_table_io_waits_summary_by_index_usage.SUM_TIMER_FETCH, "MIN_TIMER_FETCH": performance_schema_table_io_waits_summary_by_index_usage.MIN_TIMER_FETCH, "AVG_TIMER_FETCH": performance_schema_table_io_waits_summary_by_index_usage.AVG_TIMER_FETCH, "MAX_TIMER_FETCH": performance_schema_table_io_waits_summary_by_index_usage.MAX_TIMER_FETCH, "COUNT_INSERT": performance_schema_table_io_waits_summary_by_index_usage.COUNT_INSERT, "SUM_TIMER_INSERT": performance_schema_table_io_waits_summary_by_index_usage.SUM_TIMER_INSERT, "MIN_TIMER_INSERT": performance_schema_table_io_waits_summary_by_index_usage.MIN_TIMER_INSERT, "AVG_TIMER_INSERT": performance_schema_table_io_waits_summary_by_index_usage.AVG_TIMER_INSERT, "MAX_TIMER_INSERT": performance_schema_table_io_waits_summary_by_index_usage.MAX_TIMER_INSERT, "COUNT_UPDATE": performance_schema_table_io_waits_summary_by_index_usage.COUNT_UPDATE, "SUM_TIMER_UPDATE": performance_schema_table_io_waits_summary_by_index_usage.SUM_TIMER_UPDATE, "MIN_TIMER_UPDATE": performance_schema_table_io_waits_summary_by_index_usage.MIN_TIMER_UPDATE, "AVG_TIMER_UPDATE": performance_schema_table_io_waits_summary_by_index_usage.AVG_TIMER_UPDATE, "MAX_TIMER_UPDATE": performance_schema_table_io_waits_summary_by_index_usage.MAX_TIMER_UPDATE, "COUNT_DELETE": performance_schema_table_io_waits_summary_by_index_usage.COUNT_DELETE, "SUM_TIMER_DELETE": performance_schema_table_io_waits_summary_by_index_usage.SUM_TIMER_DELETE, "MIN_TIMER_DELETE": performance_schema_table_io_waits_summary_by_index_usage.MIN_TIMER_DELETE, "AVG_TIMER_DELETE": performance_schema_table_io_waits_summary_by_index_usage.AVG_TIMER_DELETE, "MAX_TIMER_DELETE": performance_schema_table_io_waits_summary_by_index_usage.MAX_TIMER_DELETE})
		}
		rows.Close()
	}

	type performance_schema_file_summary_by_instance_type struct {
		FILE_NAME                 string
		EVENT_NAME                string
		OBJECT_INSTANCE_BEGIN     string
		COUNT_STAR                string
		SUM_TIMER_WAIT            string
		MIN_TIMER_WAIT            string
		AVG_TIMER_WAIT            string
		MAX_TIMER_WAIT            string
		COUNT_READ                string
		SUM_TIMER_READ            string
		MIN_TIMER_READ            string
		AVG_TIMER_READ            string
		MAX_TIMER_READ            string
		SUM_NUMBER_OF_BYTES_READ  string
		COUNT_WRITE               string
		SUM_TIMER_WRITE           string
		MIN_TIMER_WRITE           string
		AVG_TIMER_WRITE           string
		MAX_TIMER_WRITE           string
		SUM_NUMBER_OF_BYTES_WRITE string
		COUNT_MISC                string
		SUM_TIMER_MISC            string
		MIN_TIMER_MISC            string
		AVG_TIMER_MISC            string
		MAX_TIMER_MISC            string
	}
	var performance_schema_file_summary_by_instance performance_schema_file_summary_by_instance_type

	rows, err = models.DB.Query("SELECT IFNULL(FILE_NAME, 'NULL') as FILE_NAME, IFNULL(EVENT_NAME, 'NULL') as EVENT_NAME, IFNULL(OBJECT_INSTANCE_BEGIN, 'NULL') as OBJECT_INSTANCE_BEGIN, IFNULL(COUNT_STAR, 'NULL') as COUNT_STAR, IFNULL(SUM_TIMER_WAIT, 'NULL') as SUM_TIMER_WAIT, IFNULL(MIN_TIMER_WAIT, 'NULL') as MIN_TIMER_WAIT, IFNULL(AVG_TIMER_WAIT, 'NULL') as AVG_TIMER_WAIT, IFNULL(MAX_TIMER_WAIT, 'NULL') as MAX_TIMER_WAIT, IFNULL(COUNT_READ, 'NULL') as COUNT_READ, IFNULL(SUM_TIMER_READ, 'NULL') as SUM_TIMER_READ, IFNULL(MIN_TIMER_READ, 'NULL') as MIN_TIMER_READ, IFNULL(AVG_TIMER_READ, 'NULL') as AVG_TIMER_READ, IFNULL(MAX_TIMER_READ, 'NULL') as MAX_TIMER_READ, IFNULL(SUM_NUMBER_OF_BYTES_READ, 'NULL') as SUM_NUMBER_OF_BYTES_READ, IFNULL(COUNT_WRITE, 'NULL') as COUNT_WRITE, IFNULL(SUM_TIMER_WRITE, 'NULL') as SUM_TIMER_WRITE, IFNULL(MIN_TIMER_WRITE, 'NULL') as MIN_TIMER_WRITE, IFNULL(AVG_TIMER_WRITE, 'NULL') as AVG_TIMER_WRITE, IFNULL(MAX_TIMER_WRITE, 'NULL') as MAX_TIMER_WRITE, IFNULL(SUM_NUMBER_OF_BYTES_WRITE, 'NULL') as SUM_NUMBER_OF_BYTES_WRITE, IFNULL(COUNT_MISC, 'NULL') as COUNT_MISC, IFNULL(SUM_TIMER_MISC, 'NULL') as SUM_TIMER_MISC, IFNULL(MIN_TIMER_MISC, 'NULL') as MIN_TIMER_MISC, IFNULL(AVG_TIMER_MISC, 'NULL') as AVG_TIMER_MISC, IFNULL(MAX_TIMER_MISC, 'NULL') as MAX_TIMER_MISC FROM performance_schema.file_summary_by_instance")
	if err != nil {
		DbCollectQueriesOptimization.logger.Error(err)
	} else {
		for rows.Next() {
			err := rows.Scan(&performance_schema_file_summary_by_instance.FILE_NAME, &performance_schema_file_summary_by_instance.EVENT_NAME, &performance_schema_file_summary_by_instance.OBJECT_INSTANCE_BEGIN, &performance_schema_file_summary_by_instance.COUNT_STAR, &performance_schema_file_summary_by_instance.SUM_TIMER_WAIT, &performance_schema_file_summary_by_instance.MIN_TIMER_WAIT, &performance_schema_file_summary_by_instance.AVG_TIMER_WAIT, &performance_schema_file_summary_by_instance.MAX_TIMER_WAIT, &performance_schema_file_summary_by_instance.COUNT_READ, &performance_schema_file_summary_by_instance.SUM_TIMER_READ, &performance_schema_file_summary_by_instance.MIN_TIMER_READ, &performance_schema_file_summary_by_instance.AVG_TIMER_READ, &performance_schema_file_summary_by_instance.MAX_TIMER_READ, &performance_schema_file_summary_by_instance.SUM_NUMBER_OF_BYTES_READ, &performance_schema_file_summary_by_instance.COUNT_WRITE, &performance_schema_file_summary_by_instance.SUM_TIMER_WRITE, &performance_schema_file_summary_by_instance.MIN_TIMER_WRITE, &performance_schema_file_summary_by_instance.AVG_TIMER_WRITE, &performance_schema_file_summary_by_instance.MAX_TIMER_WRITE, &performance_schema_file_summary_by_instance.SUM_NUMBER_OF_BYTES_WRITE, &performance_schema_file_summary_by_instance.COUNT_MISC, &performance_schema_file_summary_by_instance.SUM_TIMER_MISC, &performance_schema_file_summary_by_instance.MIN_TIMER_MISC, &performance_schema_file_summary_by_instance.AVG_TIMER_MISC, &performance_schema_file_summary_by_instance.MAX_TIMER_MISC)
			if err != nil {
				DbCollectQueriesOptimization.logger.Error(err)
				return err
			}
			metrics.DB.QueriesOptimization["performance_schema_file_summary_by_instance"] = append(metrics.DB.QueriesOptimization["performance_schema_file_summary_by_instance"], models.MetricGroupValue{"FILE_NAME": performance_schema_file_summary_by_instance.FILE_NAME, "EVENT_NAME": performance_schema_file_summary_by_instance.EVENT_NAME, "OBJECT_INSTANCE_BEGIN": performance_schema_file_summary_by_instance.OBJECT_INSTANCE_BEGIN, "COUNT_STAR": performance_schema_file_summary_by_instance.COUNT_STAR, "SUM_TIMER_WAIT": performance_schema_file_summary_by_instance.SUM_TIMER_WAIT, "MIN_TIMER_WAIT": performance_schema_file_summary_by_instance.MIN_TIMER_WAIT, "AVG_TIMER_WAIT": performance_schema_file_summary_by_instance.AVG_TIMER_WAIT, "MAX_TIMER_WAIT": performance_schema_file_summary_by_instance.MAX_TIMER_WAIT, "COUNT_READ": performance_schema_file_summary_by_instance.COUNT_READ, "SUM_TIMER_READ": performance_schema_file_summary_by_instance.SUM_TIMER_READ, "MIN_TIMER_READ": performance_schema_file_summary_by_instance.MIN_TIMER_READ, "AVG_TIMER_READ": performance_schema_file_summary_by_instance.AVG_TIMER_READ, "MAX_TIMER_READ": performance_schema_file_summary_by_instance.MAX_TIMER_READ, "SUM_NUMBER_OF_BYTES_READ": performance_schema_file_summary_by_instance.SUM_NUMBER_OF_BYTES_READ, "COUNT_WRITE": performance_schema_file_summary_by_instance.COUNT_WRITE, "SUM_TIMER_WRITE": performance_schema_file_summary_by_instance.SUM_TIMER_WRITE, "MIN_TIMER_WRITE": performance_schema_file_summary_by_instance.MIN_TIMER_WRITE, "AVG_TIMER_WRITE": performance_schema_file_summary_by_instance.AVG_TIMER_WRITE, "MAX_TIMER_WRITE": performance_schema_file_summary_by_instance.MAX_TIMER_WRITE, "SUM_NUMBER_OF_BYTES_WRITE": performance_schema_file_summary_by_instance.SUM_NUMBER_OF_BYTES_WRITE, "COUNT_MISC": performance_schema_file_summary_by_instance.COUNT_MISC, "SUM_TIMER_MISC": performance_schema_file_summary_by_instance.SUM_TIMER_MISC, "MIN_TIMER_MISC": performance_schema_file_summary_by_instance.MIN_TIMER_MISC, "AVG_TIMER_MISC": performance_schema_file_summary_by_instance.AVG_TIMER_MISC, "MAX_TIMER_MISC": performance_schema_file_summary_by_instance.MAX_TIMER_MISC})
		}
		rows.Close()
	}

	type information_schema_referential_constraints_type struct {
		CONSTRAINT_SCHEMA        string
		CONSTRAINT_NAME          string
		UNIQUE_CONSTRAINT_SCHEMA string
		UNIQUE_CONSTRAINT_NAME   string
		MATCH_OPTION             string
		UPDATE_RULE              string
		DELETE_RULE              string
		TABLE_NAME               string
		REFERENCED_TABLE_NAME    string
	}
	var information_schema_referential_constraints information_schema_referential_constraints_type

	rows, err = models.DB.Query("SELECT IFNULL(CONSTRAINT_SCHEMA, 'NULL') as CONSTRAINT_SCHEMA, IFNULL(CONSTRAINT_NAME, 'NULL') as CONSTRAINT_NAME, IFNULL(UNIQUE_CONSTRAINT_SCHEMA, 'NULL') as UNIQUE_CONSTRAINT_SCHEMA, IFNULL(UNIQUE_CONSTRAINT_NAME, 'NULL') as UNIQUE_CONSTRAINT_NAME, IFNULL(MATCH_OPTION, 'NULL') as MATCH_OPTION, IFNULL(UPDATE_RULE, 'NULL') as UPDATE_RULE, IFNULL(DELETE_RULE, 'NULL') as DELETE_RULE, IFNULL(TABLE_NAME, 'NULL') as TABLE_NAME, IFNULL(REFERENCED_TABLE_NAME, 'NULL') as REFERENCED_TABLE_NAME FROM information_schema.REFERENTIAL_CONSTRAINTS" + FilterQueryString(DbCollectQueriesOptimization.configuration.DatabasesQueryOptimization, "CONSTRAINT_SCHEMA"))
	if err != nil {
		DbCollectQueriesOptimization.logger.Error(err)
	} else {
		for rows.Next() {
			err := rows.Scan(&information_schema_referential_constraints.CONSTRAINT_SCHEMA, &information_schema_referential_constraints.CONSTRAINT_NAME, &information_schema_referential_constraints.UNIQUE_CONSTRAINT_SCHEMA, &information_schema_referential_constraints.UNIQUE_CONSTRAINT_NAME, &information_schema_referential_constraints.MATCH_OPTION, &information_schema_referential_constraints.UPDATE_RULE, &information_schema_referential_constraints.DELETE_RULE, &information_schema_referential_constraints.TABLE_NAME, &information_schema_referential_constraints.REFERENCED_TABLE_NAME)
			if err != nil {
				DbCollectQueriesOptimization.logger.Error(err)
				return err
			}
			metrics.DB.QueriesOptimization["information_schema_referential_constraints"] = append(metrics.DB.QueriesOptimization["information_schema_referential_constraints"], models.MetricGroupValue{"CONSTRAINT_SCHEMA": information_schema_referential_constraints.CONSTRAINT_SCHEMA, "CONSTRAINT_NAME": information_schema_referential_constraints.CONSTRAINT_NAME, "UNIQUE_CONSTRAINT_SCHEMA": information_schema_referential_constraints.UNIQUE_CONSTRAINT_SCHEMA, "UNIQUE_CONSTRAINT_NAME": information_schema_referential_constraints.UNIQUE_CONSTRAINT_NAME, "MATCH_OPTION": information_schema_referential_constraints.MATCH_OPTION, "UPDATE_RULE": information_schema_referential_constraints.UPDATE_RULE, "DELETE_RULE": information_schema_referential_constraints.DELETE_RULE, "TABLE_NAME": information_schema_referential_constraints.TABLE_NAME, "REFERENCED_TABLE_NAME": information_schema_referential_constraints.REFERENCED_TABLE_NAME})
		}
		rows.Close()
	}

	DbCollectQueriesOptimization.logger.V(5).Info("collectMetrics ", metrics.DB.Queries)
	DbCollectQueriesOptimization.logger.V(5).Info("collectMetrics ", metrics.DB.QueriesOptimization)

	return nil

}

func CollectionExplain(digests map[string]models.MetricGroupValue, field_sorting string, logger logging.Logger, configuration *config.Config, is_mysql57 bool) {
	var explain, schema_name_conn, query_text string
	var i int
	var db *sql.DB

	pairs := make([][2]interface{}, 0, len(digests))
	for k, v := range digests {
		pairs = append(pairs, [2]interface{}{k, v[field_sorting]})
	}
	//logger.Println(pairs)
	// Sort slice based on values
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i][1].(int) > pairs[j][1].(int)
	})

	for _, p := range pairs {
		k := p[0].(string)
		if i > 100 {
			break
		}

		if digests[k]["schema_name"].(string) != "mysql" && digests[k]["schema_name"].(string) != "information_schema" &&
			digests[k]["schema_name"].(string) != "performance_schema" && digests[k]["schema_name"].(string) != "NULL" &&
			(strings.Contains(digests[k]["query_text"].(string), "SELECT ") || strings.Contains(digests[k]["query_text"].(string), "select ")) &&
			digests[k]["explain"] == nil {

			if digests[k]["query_text"].(string) == "" {
				continue
			}
			if strings.Contains(digests[k]["query_text"].(string), "EXPLAIN FORMAT=JSON") {
				continue
			}
			if IsSchemaNameExclude(digests[k]["schema_name"].(string), configuration.DatabasesQueryOptimization) {
				continue
			}

			if (strings.Contains(digests[k]["query_text"].(string), "SELECT") || strings.Contains(digests[k]["query_text"].(string), "select")) &&
				strings.Contains(digests[k]["query_text"].(string), "SQL_NO_CACHE") &&
				!(strings.Contains(digests[k]["query_text"].(string), "WHERE") || strings.Contains(digests[k]["query_text"].(string), "where")) {
				logger.V(5).Info("Query From mysqldump", digests[k]["query_text"].(string))
				continue
			}

			if strings.HasSuffix(digests[k]["query_text"].(string), "...") {
				digests[k]["explain_error"] = "need_full_query"
				logger.V(5).Info("need_full_query") //, digests[k]["query_text"].(string))
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

			//Try exec EXPLAIN for origin query
			err := db.QueryRow("EXPLAIN FORMAT=JSON " + digests[k]["query_text"].(string)).Scan(&explain)
			if err != nil {
				logger.Error("Explain Error: ", err)
				if strings.Contains(err.Error(), "command denied to user") {
					digests[k]["explain_error"] = "need_grant_permission"
					continue
				} else {
					digests[k]["explain_error"] = err.Error()
					logger.Error(digests[k]["query_text"].(string))
				}
			} else {
				logger.V(5).Info(i, "OK")
				digests[k]["explain"] = explain
				i = i + 1
				continue
			}

			//Try exec EXPLAIN for  query with replace "\"" on "'"
			query_text = strings.Replace(digests[k]["query_text"].(string), "\"", "'", -1)
			err_1 := db.QueryRow("EXPLAIN FORMAT=JSON " + query_text).Scan(&explain)
			if err_1 != nil {
				logger.Error("Explain Error: ", err_1)
				if strings.Contains(err_1.Error(), "command denied to user") {
					digests[k]["explain_error"] = "need_grant_permission"
					continue
				} else {
					digests[k]["explain_error"] = err_1.Error()
					logger.Error(query_text)
				}
			} else {
				logger.V(5).Info(i, "OK")
				digests[k]["explain"] = explain
				i = i + 1
				continue
			}

			//Try exec EXPLAIN for  query with replace "\"" on "`"
			query_text = strings.Replace(digests[k]["query_text"].(string), "\"", "`", -1)
			err_2 := db.QueryRow("EXPLAIN FORMAT=JSON " + query_text).Scan(&explain)
			if err_2 != nil {
				logger.Error("Explain Error: ", err_2)
				if strings.Contains(err_2.Error(), "command denied to user") {
					digests[k]["explain_error"] = "need_grant_permission"
					continue
				} else {
					digests[k]["explain_error"] = err_2.Error()
					logger.Error(query_text)
				}
			} else {
				logger.V(5).Info(i, "OK")
				digests[k]["explain"] = explain
				i = i + 1
				continue
			}

		}
	}
}
