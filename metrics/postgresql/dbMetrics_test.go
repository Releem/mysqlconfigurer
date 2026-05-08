package postgresql

import (
	"os"
	"strings"
	"testing"
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
