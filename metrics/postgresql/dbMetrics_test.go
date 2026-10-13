package postgresql

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/metrics/tablesize"
	"github.com/Releem/mysqlconfigurer/models"
	logging "github.com/google/logger"
	"github.com/lib/pq"
)

func TestPGTableStatisticsPreservePlatformFieldsWithoutLegacyView(t *testing.T) {
	logger := logging.Init("pg-table-statistics", false, false, &bytes.Buffer{})
	defer logger.Close()
	g := NewDBMetricsConfigGatherer(*logger, &config.Config{}, false, nil, nil)
	newPGTableCacheServer(t, g.configuration, &pgTableCacheSQL{userTableStatistics: true})
	metrics := &models.Metrics{}
	metrics.DB.Metrics.Databases = []string{"app", "archive"}
	metrics.DB.Metrics.Status = make(models.MetricGroupValue)
	if err := g.GetMetrics(metrics); err != nil {
		t.Fatal(err)
	}
	rows, _ := metrics.DB.Metrics.Status["pg_stat_user_tables"].([]models.MetricGroupValue)
	if len(rows) != 2 {
		t.Fatalf("GetMetrics table statistics = %v, want one row from each database without querying the legacy view", rows)
	}
	want := models.MetricGroupValue{
		"relid": "123", "schemaname": "public", "relname": "orders",
		"seq_scan": "10", "idx_scan": "90", "n_live_tup": "1980", "n_dead_tup": "20",
		"vacuum_count": "0", "autovacuum_count": "1", "last_vacuum": nil,
		"last_autovacuum": "2026-10-13 10:00:00+00", "n_mod_since_analyze": "20",
		"last_analyze": "2026-10-13 09:50:00+00", "last_autoanalyze": "2026-10-13 09:58:00+00",
	}
	if !reflect.DeepEqual(rows[0], want) {
		t.Errorf("GetMetrics table statistics = %#v, want %#v", rows[0], want)
	}
	for _, field := range []string{"idx_scan", "last_vacuum", "last_autovacuum", "last_analyze", "last_autoanalyze"} {
		if value, present := rows[1][field]; !present || value != nil {
			t.Errorf("GetMetrics archive field %s = %v (present=%t), want SQL NULL", field, value, present)
		}
	}
	for view := range PG_STAT_PER_DB_VIEWS {
		if view == "pg_stat_user_tables" {
			continue
		}
		if rows, _ := metrics.DB.Metrics.Status[view].([]models.MetricGroupValue); len(rows) != 2 {
			t.Errorf("GetMetrics %s = %#v, want unchanged collection from both databases", view, rows)
		}
	}
}

func TestPgStatActivityQueryHandlesNullableConnectionFields(t *testing.T) {
	source, err := os.ReadFile("dbMetrics.go")
	if err != nil {
		t.Fatal(err)
	}

	query := string(source)
	if !strings.Contains(query, "COALESCE(host(client_addr), 'local')") {
		t.Fatal("client_address should not become NULL when client_addr is NULL")
	}

	if !strings.Contains(query, "CONCAT_WS(' ', wait_event_type::text, wait_event::text) AS wait_event") {
		t.Fatal("wait_event should preserve partial wait event data when one column is NULL")
	}

	if !strings.Contains(query, "WHERE state IS NOT NULL") {
		t.Fatal("pg_stat_activity query should filter rows with incomplete process state")
	}
}

func TestPGMetricsReusesPooledConnectionsAndContinuesAfterActivityError(t *testing.T) {
	var output bytes.Buffer
	g := NewDBMetricsConfigGatherer(*logging.Init("pg-database-lifecycle", false, false, &output), &config.Config{}, true, nil, nil)
	fake := &pgTableCacheSQL{failStatisticsDatabase: "broken"}
	server := newPGTableCacheServer(t, g.configuration, fake)
	m := &models.Metrics{}
	m.DB.Metrics.Databases = []string{"broken", "healthy"}
	m.DB.Metrics.Status = make(models.MetricGroupValue)
	if err := g.GetMetrics(m); err != nil {
		t.Fatal(err)
	}
	for view := range PG_STAT_PER_DB_VIEWS {
		if rows, _ := m.DB.Metrics.Status[view].([]models.MetricGroupValue); len(rows) != 1 {
			t.Fatalf("healthy database statistics for %s = %#v", view, rows)
		}
	}
	next := &models.Metrics{}
	next.DB.Metrics.Databases = m.DB.Metrics.Databases
	next.DB.Metrics.Status = make(models.MetricGroupValue)
	if err := g.GetMetrics(next); err != nil {
		t.Fatal(err)
	}
	// Statistics, table sizes, and the next collection share one handle per database.
	if got := strings.Join(server.openedDatabases(), ","); got != "broken,healthy" {
		t.Fatalf("each database should be opened once and reused, got %s", got)
	}
	g.connections.Close()
	server.waitForClosed(t, 2)
	if got := strings.Count(output.String(), "pg_stat_user_tables unavailable"); got != 2*len(PG_STAT_PER_DB_VIEWS) {
		t.Fatalf("activity errors logged %d times, want %d", got, 2*len(PG_STAT_PER_DB_VIEWS))
	}
}

func TestPGTableSizesDiscardSnapshotAfterAggregateError(t *testing.T) {
	fake := &pgTableCacheSQL{failTableQuery: 2} // Aggregate query in the second database.
	g := NewDBMetricsConfigGatherer(*logging.Init("pg-table-count-error", false, false, &bytes.Buffer{}), &config.Config{}, true, nil, nil)
	newPGTableCacheServer(t, g.configuration, fake)
	value, err := g.collectTableSizes([]string{"app", "archive"})
	if err == nil {
		t.Fatal("aggregate error should be returned")
	}
	if value != nil {
		t.Fatalf("aggregate failure returned a partial snapshot: %+v", value)
	}
	fake.setFailure(false, 0)
	value, err = g.collectTableSizes([]string{"app", "archive"})
	if err != nil {
		t.Fatal(err)
	}
	if tablesize.EngineTableCount(value) != 4 {
		t.Fatalf("retry should count each table once, got %d", tablesize.EngineTableCount(value))
	}
}

func TestPGTableSizesReleaseHandleAfterAggregateError(t *testing.T) {
	fake := &pgTableCacheSQL{failTableQuery: 1}
	g := NewDBMetricsConfigGatherer(*logging.Init("pg-table-size-release", false, false, &bytes.Buffer{}), &config.Config{}, true, nil, nil)
	server := newPGTableCacheServer(t, g.configuration, fake)
	if _, err := g.collectTableSizes([]string{"app"}); err == nil {
		t.Fatal("aggregate error should be returned")
	}
	// A handle still referenced by the failed refresh would survive Close.
	g.connections.Close()
	server.waitForClosed(t, 1)
}

func TestDbMetricsUsesCapabilityInfoRelation(t *testing.T) {
	source, err := os.ReadFile("dbMetrics.go")
	if err != nil {
		t.Fatal(err)
	}
	code := string(source)
	if !strings.Contains(code, "capabilities.PgStatStatementsInfoRelation") ||
		!strings.Contains(code, `FROM "+capabilities.PgStatStatementsInfoRelation`) {
		t.Fatal("pg_stat_statements_info query must use the centrally detected extension relation")
	}
}

func TestIsPGManagedInternalDatabase(t *testing.T) {
	tests := []struct {
		name         string
		instanceType string
		database     string
		want         bool
	}{
		{name: "local rdsadmin database is collected", instanceType: "local", database: "rdsadmin", want: false},
		{name: "local cloudsqladmin database is collected", instanceType: "local", database: "cloudsqladmin", want: false},
		{name: "local azure maintenance database is collected", instanceType: "local", database: "azure_maintenance", want: false},
		{name: "AWS skips its maintenance database", instanceType: "aws/rds", database: "rdsadmin", want: true},
		{name: "AWS collects another provider maintenance name", instanceType: "aws/rds", database: "cloudsqladmin", want: false},
		{name: "GCP skips its maintenance database", instanceType: "gcp/cloudsql", database: "cloudsqladmin", want: true},
		{name: "Azure PostgreSQL skips its maintenance database", instanceType: "azure/postgresql", database: "azure_maintenance", want: true},
		{name: "case and padding are ignored", instanceType: " AWS/RDS ", database: "  RDSAdmin ", want: true},
		{name: "customer database is collected", instanceType: "aws/rds", database: "app", want: false},
		{name: "similar customer name is not skipped", instanceType: "aws/rds", database: "rdsadmin_reports", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isPGManagedInternalDatabase(tt.instanceType, tt.database); got != tt.want {
				t.Errorf("isPGManagedInternalDatabase(%q, %q) = %t, want %t", tt.instanceType, tt.database, got, tt.want)
			}
		})
	}
}

func TestPgHBARulesUnavailable(t *testing.T) {
	tests := []struct {
		name         string
		instanceType string
		err          error
		want         bool
	}{
		{
			name:         "Aurora denies the rdsadmin-owned view",
			instanceType: "aws/rds",
			err:          &pq.Error{Code: "42501", Message: `permission denied for view pg_hba_file_rules`},
			want:         true,
		},
		{
			name:         "local PostgreSQL reports insufficient privileges",
			instanceType: "local",
			err:          &pq.Error{Code: "42501", Message: `permission denied for view pg_hba_file_rules`},
			want:         false,
		},
		{
			name:         "PostgreSQL below 10 has no such view",
			instanceType: "local",
			err:          &pq.Error{Code: "42P01", Message: `relation "pg_hba_file_rules" does not exist`},
			want:         true,
		},
		{
			name:         "wrapped managed permission error is still recognised",
			instanceType: "aws/rds",
			err:          fmt.Errorf("collect pg_hba: %w", &pq.Error{Code: "42501"}),
			want:         true,
		},
		{
			name:         "a real query fault must stay an error",
			instanceType: "aws/rds",
			err:          &pq.Error{Code: "57014", Message: "canceling statement due to statement timeout"},
			want:         false,
		},
		{
			name:         "a non-PostgreSQL error must stay an error",
			instanceType: "aws/rds",
			err:          errors.New("connection reset by peer"),
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := pgHBARulesUnavailable(tt.instanceType, tt.err); got != tt.want {
				t.Errorf("pgHBARulesUnavailable(%q, %v) = %t, want %t", tt.instanceType, tt.err, got, tt.want)
			}
		})
	}
}

func TestDBInfoHandlePgHBARulesUnavailableWarnsOnlyForExpectedErrors(t *testing.T) {
	permissionErr := &pq.Error{Code: "42501", Message: `permission denied for view pg_hba_file_rules`}
	tests := []struct {
		name         string
		instanceType string
		wantHandled  bool
		wantWarning  bool
	}{
		{name: "managed provider warns and suppresses", instanceType: "aws/rds", wantHandled: true, wantWarning: true},
		{name: "local installation leaves permission error actionable", instanceType: "local", wantHandled: false, wantWarning: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			logger := *logging.Init("pg-hba-unavailable-test", false, false, &output)
			gatherer := NewDBInfoGatherer(logger, &config.Config{InstanceType: tt.instanceType}, nil)

			gotHandled := gatherer.handlePgHBARulesUnavailable(permissionErr)
			if gotHandled != tt.wantHandled {
				t.Errorf("handlePgHBARulesUnavailable(%q, %v) = %t, want %t", tt.instanceType, permissionErr, gotHandled, tt.wantHandled)
			}
			gotWarning := strings.Contains(output.String(), "pg_hba_file_rules is unavailable")
			if gotWarning != tt.wantWarning {
				t.Errorf("handlePgHBARulesUnavailable(%q, %v) warning = %t, want %t; output=%q", tt.instanceType, permissionErr, gotWarning, tt.wantWarning, output.String())
			}
		})
	}
}
