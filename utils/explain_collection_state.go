package utils

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Releem/mysqlconfigurer/config"
	logging "github.com/google/logger"
)

// ExplainCollectionState deduplicates EXPLAIN attempts, records per-database
// connection failures, and holds the connections of one collection pass.
// Per-database handles come from a DatabaseConnectionPool, so ranked queries that
// alternate between databases do not reconnect on every switch. Session holds a
// single pinned connection for engines that switch databases with USE.
type ExplainCollectionState struct {
	attempted       map[string]struct{}
	failedDatabases map[string]string
	Connect         func(*config.Config, logging.Logger, string) (*sql.DB, error)

	pool     *DatabaseConnectionPool
	ownsPool bool
	current  struct {
		database string
		db       *sql.DB
		release  func()
	}

	sessionDB  *sql.DB
	session    *sql.Conn
	sessionErr error
	// SessionDatabase is the default database last selected on the pinned
	// session; ResetSession clears it because a new connection has none.
	SessionDatabase string
}

// NewExplainCollectionState creates state with a private pool closed by Close.
func NewExplainCollectionState() *ExplainCollectionState {
	state := NewExplainCollectionStateWithPool(NewDatabaseConnectionPool(config.DefaultPgMaxDatabaseConnections))
	state.ownsPool = true
	return state
}

// NewExplainCollectionStateWithPool borrows handles from a pool that outlives
// the collection pass; Close releases them without closing the pool.
func NewExplainCollectionStateWithPool(pool *DatabaseConnectionPool) *ExplainCollectionState {
	return &ExplainCollectionState{
		attempted: make(map[string]struct{}),
		Connect:   ConnectionDatabaseErr,
		pool:      pool,
	}
}

func (state *ExplainCollectionState) Close() {
	if state == nil {
		return
	}
	state.releaseCurrent()
	if state.ownsPool {
		state.pool.Close()
	}
	if state.session != nil {
		_ = state.session.Close()
		state.session = nil
	}
	if state.sessionDB != nil {
		_ = state.sessionDB.Close()
		state.sessionDB = nil
	}
}

func (state *ExplainCollectionState) releaseCurrent() {
	if state.current.release != nil {
		state.current.release()
	}
	state.current.database, state.current.db, state.current.release = "", nil, nil
}

func (state *ExplainCollectionState) TryBeginAttempt(cacheKey string) bool {
	if state == nil || cacheKey == "" {
		return false
	}
	if _, exists := state.attempted[cacheKey]; exists {
		return false
	}
	state.attempted[cacheKey] = struct{}{}
	return true
}

func (state *ExplainCollectionState) DatabaseFailed(database string) bool {
	if state == nil || state.failedDatabases == nil {
		return false
	}
	_, failed := state.failedDatabases[database]
	return failed
}

func (state *ExplainCollectionState) FailedReason(database string) string {
	if state == nil || state.failedDatabases == nil {
		return ""
	}
	return state.failedDatabases[database]
}

func (state *ExplainCollectionState) MarkDatabaseFailed(database, reason string) {
	if state == nil {
		return
	}
	if state.failedDatabases == nil {
		state.failedDatabases = make(map[string]string)
	}
	if reason == "" {
		reason = "connection failed"
	}
	state.failedDatabases[database] = reason
}

// Connection returns the pooled handle for database. Only the latest handle is
// held by the state; the pool keeps earlier ones open for later switches.
func (state *ExplainCollectionState) Connection(database string, connect func() (*sql.DB, error)) (*sql.DB, error) {
	if state == nil {
		return nil, fmt.Errorf("explain collection state is nil")
	}
	if state.DatabaseFailed(database) {
		return nil, fmt.Errorf("%s", state.FailedReason(database))
	}
	if state.current.db != nil && state.current.database == database {
		return state.current.db, nil
	}
	state.releaseCurrent()
	db, release, err := state.pool.Acquire(database, connect)
	if db == nil {
		reason := "connection failed"
		if err != nil {
			reason = err.Error()
		}
		state.MarkDatabaseFailed(database, reason)
		if err == nil {
			err = fmt.Errorf("%s", reason)
		}
		return nil, err
	}
	state.current.database, state.current.db, state.current.release = database, db, release
	return db, nil
}

// Session returns one connection pinned for the whole pass, so session state
// such as the default database set by USE applies to every later query. A
// failed connect is cached for the rest of the pass; a failure to pin a
// connection from an open handle, for example while reconnecting after
// ResetSession, is not, so the next call retries.
func (state *ExplainCollectionState) Session(connect func() (*sql.DB, error)) (*sql.Conn, error) {
	if state == nil {
		return nil, fmt.Errorf("explain collection state is nil")
	}
	if state.session != nil {
		return state.session, nil
	}
	if state.sessionErr != nil {
		return nil, state.sessionErr
	}
	if state.sessionDB == nil {
		db, err := connect()
		if db == nil {
			if err == nil {
				err = fmt.Errorf("connection failed")
			}
			state.sessionErr = err
			return nil, err
		}
		state.sessionDB = db
	}
	conn, err := state.sessionDB.Conn(context.Background())
	if err != nil {
		return nil, err
	}
	state.session = conn
	return conn, nil
}

// ResetSession drops the pinned connection after a connection-level failure;
// database/sql closes a *sql.Conn for good once it reports driver.ErrBadConn.
// The next Session call pins a fresh connection from the same handle, which
// discards broken connections and reconnects.
func (state *ExplainCollectionState) ResetSession() {
	if state == nil || state.session == nil {
		return
	}
	_ = state.session.Close()
	state.session = nil
	state.SessionDatabase = ""
}

func ConnectionFailedExplainError(reason string) string {
	if reason == "" {
		reason = "connection failed"
	}
	return "connection_failed: " + reason
}
