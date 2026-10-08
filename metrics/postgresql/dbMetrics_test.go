package postgresql

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/metrics/tablesize"
	"github.com/Releem/mysqlconfigurer/models"
	logging "github.com/google/logger"
	"github.com/lib/pq"
)

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

func TestPGMetricsClosesConnectionsAndContinuesAfterActivityError(t *testing.T) {
	var output bytes.Buffer
	g := NewDBMetricsConfigGatherer(*logging.Init("pg-database-lifecycle", false, false, &output), &config.Config{}, true, nil)
	fake := &pgTableCacheSQL{failStatisticsDatabase: "broken"}
	server := newPGTableCacheServer(t, g.configuration, fake)
	m := &models.Metrics{}
	m.DB.Metrics.Databases = []string{"broken", "healthy"}
	m.DB.Metrics.Status = make(models.MetricGroupValue)
	if err := g.GetMetrics(m); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(server.openedDatabases(), ","); got != "broken,healthy,broken,healthy" {
		t.Fatalf("activity and table sizes should continue with the next database, got %s", got)
	}
	server.waitForClosed(t, 4)
	for _, view := range PG_STAT_PER_DB_VIEWS {
		if rows, _ := m.DB.Metrics.Status[view].([]models.MetricGroupValue); len(rows) != 1 {
			t.Fatalf("healthy database statistics for %s = %#v", view, rows)
		}
	}
	if got := strings.Count(output.String(), "pg_stat_user_tables unavailable"); got != len(PG_STAT_PER_DB_VIEWS) {
		t.Fatalf("activity errors logged %d times, want %d", got, len(PG_STAT_PER_DB_VIEWS))
	}
}

func TestPGTableSizesDiscardSnapshotAfterAggregateError(t *testing.T) {
	fake := &pgTableCacheSQL{failTableQuery: 2} // Aggregate query in the second database.
	g := NewDBMetricsConfigGatherer(*logging.Init("pg-table-count-error", false, false, &bytes.Buffer{}), &config.Config{}, true, nil)
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
			gatherer := NewDBInfoGatherer(logger, &config.Config{InstanceType: tt.instanceType})

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
