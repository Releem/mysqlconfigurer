package mysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	u "github.com/Releem/mysqlconfigurer/utils"
	logging "github.com/google/logger"
)

func TestCollectExplainCachesConnectionFailurePerSchema(t *testing.T) {
	logger := *logging.Init("mysql-explain-connection-failure-test", false, false, io.Discard)
	defer logger.Close()

	now := float64(time.Now().Unix())
	digests := map[string]models.MetricGroupValue{
		"blocked1": {
			"schema_name": "blocked", "query_id": "1", "query_text": "SELECT 1",
			"sum_time_us": 3, "avg_time_us": 3, "LAST_SEEN": now,
		},
		"blocked2": {
			"schema_name": "blocked", "query_id": "2", "query_text": "SELECT 2",
			"sum_time_us": 2, "avg_time_us": 2, "LAST_SEEN": now,
		},
	}

	state := u.NewExplainCollectionState()
	defer state.Close()
	connects := 0
	state.Connect = func(*config.Config, logging.Logger, string) (*sql.DB, error) {
		connects++
		return nil, errors.New("dial tcp: connection refused")
	}

	CollectExplain(digests, "sum_time_us", logger, &config.Config{}, state)

	if connects != 1 {
		t.Fatalf("blocked schema connects = %d, want 1", connects)
	}
	want := "connection_failed: dial tcp: connection refused"
	for _, key := range []string{"blocked1", "blocked2"} {
		if digests[key]["explain_error"] != want {
			t.Fatalf("%s explain_error = %#v, want %q", key, digests[key]["explain_error"], want)
		}
	}
}

func TestCollectExplainUsesOneSessionAndSwitchesSchemasWithUse(t *testing.T) {
	logger := *logging.Init("mysql-explain-session-test", false, false, io.Discard)
	defer logger.Close()

	now := float64(time.Now().Unix())
	digests := map[string]models.MetricGroupValue{}
	// Descending sum_time_us alternates schemas; `blocked` fails USE.
	for i, schema := range []string{"app_a", "blocked", "app_b", "app_a", "blocked", "app_b"} {
		key := fmt.Sprintf("%s%d", schema, i)
		digests[key] = models.MetricGroupValue{
			"schema_name": schema, "query_id": key, "query_text": "SELECT * FROM t WHERE id = 1",
			"sum_time_us": 10 - i, "avg_time_us": 10 - i, "LAST_SEEN": now,
		}
	}

	session := &mysqlExplainSessionConnector{useErrors: map[string]error{"`blocked`": errors.New("Error 1044: Access denied for user 'releem' to database 'blocked'")}}
	state := u.NewExplainCollectionState()
	connects := 0
	var handle *sql.DB
	state.Connect = func(_ *config.Config, _ logging.Logger, schema string) (*sql.DB, error) {
		connects++
		if schema != "" {
			t.Fatalf("session must not connect to schema %q", schema)
		}
		handle = sql.OpenDB(session)
		return handle, nil
	}

	CollectExplain(digests, "sum_time_us", logger, &config.Config{}, state)

	if connects != 1 || session.opened != 1 {
		t.Fatalf("connects = %d, server connections = %d; want one session", connects, session.opened)
	}
	if got, want := strings.Join(session.uses, ","), "`app_a`,`blocked`,`app_b`,`app_a`,`app_b`"; got != want {
		t.Fatalf("USE statements = %s, want %s", got, want)
	}
	for key, digest := range digests {
		if digest["schema_name"] == "blocked" {
			want := "connection_failed: Error 1044: Access denied for user 'releem' to database 'blocked'"
			if digest["explain_error"] != want || digest["explain"] != nil {
				t.Fatalf("%s explain_error = %#v, want %q", key, digest["explain_error"], want)
			}
			continue
		}
		if digest["explain"] == nil {
			t.Fatalf("%s has no explain: %#v", key, digest)
		}
	}
	state.Close()
	if err := handle.Ping(); err == nil {
		t.Fatal("Close must close the session handle")
	}
}

func TestQuoteMySQLIdentifierEscapesBackticks(t *testing.T) {
	if got := quoteMySQLIdentifier("we`ird"); got != "`we``ird`" {
		t.Fatalf("quoteMySQLIdentifier = %s", got)
	}
}

type mysqlExplainSessionConnector struct {
	useErrors map[string]error
	uses      []string
	opened    int
	// dropOnUse breaks the first connection on that USE, as a dropped server
	// connection does.
	dropOnUse string
	// failConnects fails that many reconnects after the first connection.
	failConnects int
	// dropOnExplain breaks the first connection on its first EXPLAIN.
	dropOnExplain bool
	explains      int
}

func (connector *mysqlExplainSessionConnector) Connect(context.Context) (driver.Conn, error) {
	if connector.opened > 0 && connector.failConnects > 0 {
		connector.failConnects--
		return nil, errors.New("dial tcp: i/o timeout")
	}
	connector.opened++
	return &mysqlExplainSessionConn{connector: connector}, nil
}

func (connector *mysqlExplainSessionConnector) Driver() driver.Driver { return mysqlSchemaTestDriver{} }

type mysqlExplainSessionConn struct {
	connector *mysqlExplainSessionConnector
}

func (connection *mysqlExplainSessionConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not supported by the explain session test driver")
}

func (connection *mysqlExplainSessionConn) Close() error { return nil }

func (connection *mysqlExplainSessionConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions are not supported by the explain session test driver")
}

func (connection *mysqlExplainSessionConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	schema, ok := strings.CutPrefix(query, "USE ")
	if !ok {
		return nil, fmt.Errorf("unexpected exec %q", query)
	}
	connection.connector.uses = append(connection.connector.uses, schema)
	if connection.connector.dropOnUse == schema && connection.connector.opened == 1 {
		return nil, driver.ErrBadConn
	}
	if err := connection.connector.useErrors[schema]; err != nil {
		return nil, err
	}
	return driver.RowsAffected(0), nil
}

func (connection *mysqlExplainSessionConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if !strings.HasPrefix(query, "EXPLAIN FORMAT=JSON ") {
		return nil, fmt.Errorf("unexpected query %q", query)
	}
	connection.connector.explains++
	if connection.connector.dropOnExplain && connection.connector.opened == 1 && connection.connector.explains == 1 {
		return nil, driver.ErrBadConn
	}
	return mysqlSchemaRows([]driver.Value{`{"query_block":{}}`}), nil
}

func TestCollectExplainReplacesSessionAfterDroppedConnection(t *testing.T) {
	logger := *logging.Init("mysql-explain-session-drop-test", false, false, io.Discard)
	defer logger.Close()

	now := float64(time.Now().Unix())
	digests := map[string]models.MetricGroupValue{}
	for i, schema := range []string{"app_a", "app_b", "app_c"} {
		key := fmt.Sprintf("%s%d", schema, i)
		digests[key] = models.MetricGroupValue{
			"schema_name": schema, "query_id": key, "query_text": "SELECT * FROM t WHERE id = 1",
			"sum_time_us": 10 - i, "avg_time_us": 10 - i, "LAST_SEEN": now,
		}
	}

	session := &mysqlExplainSessionConnector{dropOnUse: "`app_b`"}
	state := u.NewExplainCollectionState()
	defer state.Close()
	connects := 0
	state.Connect = func(*config.Config, logging.Logger, string) (*sql.DB, error) {
		connects++
		return sql.OpenDB(session), nil
	}

	CollectExplain(digests, "sum_time_us", logger, &config.Config{}, state)

	if connects != 1 || session.opened != 2 {
		t.Fatalf("connects = %d, server connections = %d; want one handle reconnecting once", connects, session.opened)
	}
	if got, want := strings.Join(session.uses, ","), "`app_a`,`app_b`,`app_b`,`app_c`"; got != want {
		t.Fatalf("USE statements = %s, want %s", got, want)
	}
	for key, digest := range digests {
		if digest["explain"] == nil || digest["explain_error"] != nil {
			t.Fatalf("%s must be explained after reconnect: %#v", key, digest)
		}
	}
}

func TestCollectExplainSkipsUseForCurrentSchema(t *testing.T) {
	logger := *logging.Init("mysql-explain-same-schema-test", false, false, io.Discard)
	defer logger.Close()

	now := float64(time.Now().Unix())
	digests := map[string]models.MetricGroupValue{}
	for i, schema := range []string{"app_a", "app_a", "app_b", "app_b", "app_a"} {
		key := fmt.Sprintf("%s%d", schema, i)
		digests[key] = models.MetricGroupValue{
			"schema_name": schema, "query_id": key, "query_text": "SELECT * FROM t WHERE id = 1",
			"sum_time_us": 10 - i, "avg_time_us": 10 - i, "LAST_SEEN": now,
		}
	}
	session := &mysqlExplainSessionConnector{}
	state := u.NewExplainCollectionState()
	defer state.Close()
	state.Connect = func(*config.Config, logging.Logger, string) (*sql.DB, error) { return sql.OpenDB(session), nil }

	CollectExplain(digests, "sum_time_us", logger, &config.Config{}, state)

	if got, want := strings.Join(session.uses, ","), "`app_a`,`app_b`,`app_a`"; got != want {
		t.Fatalf("USE statements = %s, want %s", got, want)
	}
}

func TestCollectExplainRetriesSessionAfterFailedReconnect(t *testing.T) {
	logger := *logging.Init("mysql-explain-failed-reconnect-test", false, false, io.Discard)
	defer logger.Close()

	now := float64(time.Now().Unix())
	digests := map[string]models.MetricGroupValue{}
	for i, schema := range []string{"app_a", "app_b", "app_c"} {
		key := fmt.Sprintf("%s%d", schema, i)
		digests[key] = models.MetricGroupValue{
			"schema_name": schema, "query_id": key, "query_text": "SELECT * FROM t WHERE id = 1",
			"sum_time_us": 10 - i, "avg_time_us": 10 - i, "LAST_SEEN": now,
		}
	}
	// app_b drops the connection and its reconnect times out; app_c must retry.
	session := &mysqlExplainSessionConnector{dropOnUse: "`app_b`", failConnects: 1}
	state := u.NewExplainCollectionState()
	defer state.Close()
	state.Connect = func(*config.Config, logging.Logger, string) (*sql.DB, error) { return sql.OpenDB(session), nil }

	CollectExplain(digests, "sum_time_us", logger, &config.Config{}, state)

	if digests["app_c2"]["explain"] == nil {
		t.Fatalf("a failed reconnect must not fail later schemas: %#v", digests["app_c2"])
	}
	if digests["app_b1"]["explain_error"] == nil {
		t.Fatalf("app_b must report the failed reconnect: %#v", digests["app_b1"])
	}
}

func TestCollectExplainReconnectsAfterConnectionDropsDuringExplain(t *testing.T) {
	logger := *logging.Init("mysql-explain-drop-during-explain-test", false, false, io.Discard)
	defer logger.Close()

	now := float64(time.Now().Unix())
	digests := map[string]models.MetricGroupValue{}
	for i := range 3 {
		key := fmt.Sprintf("app_a%d", i)
		digests[key] = models.MetricGroupValue{
			"schema_name": "app_a", "query_id": key, "query_text": "SELECT * FROM t WHERE id = 1",
			"sum_time_us": 10 - i, "avg_time_us": 10 - i, "LAST_SEEN": now,
		}
	}
	session := &mysqlExplainSessionConnector{dropOnExplain: true}
	state := u.NewExplainCollectionState()
	defer state.Close()
	state.Connect = func(*config.Config, logging.Logger, string) (*sql.DB, error) { return sql.OpenDB(session), nil }

	CollectExplain(digests, "sum_time_us", logger, &config.Config{}, state)

	if session.opened != 2 {
		t.Fatalf("server connections = %d, want a reconnect after the drop", session.opened)
	}
	if got, want := strings.Join(session.uses, ","), "`app_a`,`app_a`"; got != want {
		t.Fatalf("USE statements = %s, want the schema reselected on the new connection: %s", got, want)
	}
	for key, digest := range digests {
		if digest["explain"] == nil || digest["explain_error"] != nil {
			t.Fatalf("%s must be explained after reconnect: %#v", key, digest)
		}
	}
}
