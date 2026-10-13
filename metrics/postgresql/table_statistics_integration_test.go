package postgresql

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
	logging "github.com/google/logger"
	"github.com/lib/pq"
)

func TestPGTableStatisticsMatchViewAgainstPostgres(t *testing.T) {
	db := openIntegrationPostgres(t)
	schema := fmt.Sprintf("releem_stats_%d", time.Now().UnixNano())
	otherSchema := schema + "_other"
	for _, name := range []string{schema, otherSchema} {
		if _, err := db.Exec("CREATE SCHEMA " + pq.QuoteIdentifier(name)); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := db.Exec("DROP SCHEMA " + pq.QuoteIdentifier(name) + " CASCADE"); err != nil {
				t.Error(err)
			}
		})
	}
	if _, err := db.Exec(fmt.Sprintf(`
CREATE TABLE %[1]s.items (id integer PRIMARY KEY, value text);
CREATE INDEX ON %[1]s.items (value);
INSERT INTO %[1]s.items SELECT id, id::text FROM generate_series(1, 100) id;
CREATE TABLE %[1]s.empty_table (id integer);
CREATE TABLE %[1]s.events (id integer) PARTITION BY RANGE (id);
CREATE TABLE %[1]s.events_part PARTITION OF %[1]s.events FOR VALUES FROM (0) TO (100);
CREATE MATERIALIZED VIEW %[1]s.summary AS SELECT count(*) FROM %[1]s.items;
CREATE VIEW %[1]s.items_view AS SELECT * FROM %[1]s.items;
CREATE TABLE %[2]s.items (id integer);
`, pq.QuoteIdentifier(schema), pq.QuoteIdentifier(otherSchema))); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("VACUUM ANALYZE " + pq.QuoteIdentifier(schema) + ".items"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("SELECT pg_stat_force_next_flush()"); err != nil {
		t.Fatal(err)
	}
	logger := logging.Init("pg-table-statistics-integration", false, false, io.Discard)
	defer logger.Close()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec("SET LOCAL stats_fetch_consistency = 'snapshot'"); err != nil {
		t.Fatal(err)
	}
	read := func(query string) []models.MetricGroupValue {
		t.Helper()
		rows, err := tx.Query(query)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		result := utils.ScanRows(rows, *logger)
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return result
	}
	// Eleven optimizer inputs plus table identity, projected from the original view.
	legacy := read(`SELECT relid, schemaname, relname, seq_scan, idx_scan, n_live_tup, n_dead_tup,
vacuum_count, autovacuum_count, last_vacuum, last_autovacuum, n_mod_since_analyze,
last_analyze, last_autoanalyze FROM pg_stat_user_tables ORDER BY relid`)
	actual := read("SELECT * FROM (" + pgUserTableStatisticsQuery + ") statistics ORDER BY relid")
	if !reflect.DeepEqual(actual, legacy) {
		t.Fatalf("direct table statistics = %#v, want original view projection %#v", actual, legacy)
	}
	fixtures := 0
	for _, row := range actual {
		if row["schemaname"] == schema || row["schemaname"] == otherSchema {
			fixtures++
		}
		if row["schemaname"] == schema && row["relname"] == "empty_table" && row["idx_scan"] != nil {
			t.Errorf("table without indexes idx_scan = %v, want NULL", row["idx_scan"])
		}
	}
	if fixtures != 6 {
		t.Fatalf("collected fixture relations = %d, want 6 including partitions and materialized view", fixtures)
	}
}
