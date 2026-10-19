package postgresql

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/Releem/mysqlconfigurer/models"
)

type PostgresQueryStats struct {
	Datname         string
	QueryID         string
	Calls           uint64
	TotalExecTimeUS float64
	MeanExecTimeUS  float64
}

type PostgresQueryDetail struct {
	PostgresQueryStats
	Query        string
	Rows         uint64
	Explain      string
	ExplainError string
}

type postgresQueryProfile uint8

const (
	postgresQueryLightweight postgresQueryProfile = iota
	postgresQueryFull
)

func (stats PostgresQueryStats) metricGroupValue() models.MetricGroupValue {
	return models.MetricGroupValue{
		"datname":            stats.Datname,
		"queryid":            stats.QueryID,
		"calls":              stats.Calls,
		"total_exec_time_us": stats.TotalExecTimeUS,
		"mean_exec_time_us":  stats.MeanExecTimeUS,
	}
}

func (detail PostgresQueryDetail) metricGroupValue() models.MetricGroupValue {
	metric := detail.PostgresQueryStats.metricGroupValue()
	metric["query"] = detail.Query
	metric["rows"] = detail.Rows
	if detail.Explain != "" {
		metric["explain"] = detail.Explain
	}
	if detail.ExplainError != "" {
		metric["explain_error"] = detail.ExplainError
	}
	return metric
}

func pgStatStatementsQuery(capabilities PostgresCapabilitySnapshot, profile postgresQueryProfile) string {
	queryColumn := ""
	rowsExpression := ""
	if profile == postgresQueryFull {
		queryColumn = "\n\tmin(s.query) AS query,"

		rowsExpression = ",\n\t0::bigint AS rows"
		if capabilities.HasRows {
			rowsExpression = ",\n\tsum(s.rows)::bigint AS rows"
		}
	}
	return fmt.Sprintf(`
SELECT
	COALESCE(d.datname, 'NULL') AS datname,
	s.queryid::text AS queryid,%s
	sum(s.calls)::bigint AS calls,
	(sum(s.%s) * 1000)::double precision AS total_exec_time_us,
	(COALESCE(sum(s.%s) / NULLIF(sum(s.calls), 0), 0) * 1000)::double precision AS mean_exec_time_us%s
FROM %s s
LEFT JOIN pg_database d ON d.oid = s.dbid
WHERE s.queryid IS NOT NULL
GROUP BY d.datname, s.queryid`, queryColumn, capabilities.TimingColumn, capabilities.TimingColumn, rowsExpression, capabilities.PgStatStatementsRelation)
}

func collectPostgresQueryStats(ctx context.Context, db *sql.DB, capabilities PostgresCapabilitySnapshot) ([]PostgresQueryStats, error) {
	return queryContextRows(ctx, db, pgStatStatementsQuery(capabilities, postgresQueryLightweight), nil, func(rows *sql.Rows) (PostgresQueryStats, error) {
		var result PostgresQueryStats
		err := rows.Scan(&result.Datname, &result.QueryID, &result.Calls, &result.TotalExecTimeUS, &result.MeanExecTimeUS)
		result.QueryID = normalizePgQueryID(result.QueryID)
		return result, err
	})
}

func collectPostgresQueryDetails(ctx context.Context, db *sql.DB, capabilities PostgresCapabilitySnapshot) ([]PostgresQueryDetail, error) {
	return queryContextRows(ctx, db, pgStatStatementsQuery(capabilities, postgresQueryFull), nil, func(rows *sql.Rows) (PostgresQueryDetail, error) {
		var result PostgresQueryDetail
		err := rows.Scan(&result.Datname, &result.QueryID, &result.Query, &result.Calls, &result.TotalExecTimeUS, &result.MeanExecTimeUS, &result.Rows)
		result.QueryID = normalizePgQueryID(result.QueryID)
		return result, err
	})
}

func postgresQueryStatsMetrics(rows []PostgresQueryStats) []models.MetricGroupValue {
	result := make([]models.MetricGroupValue, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.metricGroupValue())
	}
	return result
}

func postgresQueryDetailMetrics(rows []PostgresQueryDetail) []models.MetricGroupValue {
	result := make([]models.MetricGroupValue, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.metricGroupValue())
	}
	return result
}

func postgresQueryDetailMap(rows []PostgresQueryDetail) map[string]PostgresQueryDetail {
	result := make(map[string]PostgresQueryDetail, len(rows))
	for _, row := range rows {
		result[pgDigestKey(row.Datname, row.QueryID)] = row
	}
	return result
}

func sortedPostgresQueryDetails(rows map[string]PostgresQueryDetail) []PostgresQueryDetail {
	result := make([]PostgresQueryDetail, 0, len(rows))
	for _, row := range rows {
		result = append(result, row)
	}
	return result
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
