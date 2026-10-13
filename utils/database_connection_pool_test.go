package utils

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync"
	"testing"
)

type poolTestConnector struct{}

func (poolTestConnector) Connect(context.Context) (driver.Conn, error) { return poolTestConn{}, nil }
func (poolTestConnector) Driver() driver.Driver                        { return poolTestDriver{} }

type poolTestDriver struct{}

func (poolTestDriver) Open(string) (driver.Conn, error) { return poolTestConn{}, nil }

type poolTestConn struct{}

func (poolTestConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("unsupported") }
func (poolTestConn) Close() error                        { return nil }
func (poolTestConn) Begin() (driver.Tx, error)           { return nil, errors.New("unsupported") }

type poolTestConnects struct {
	mu     sync.Mutex
	counts map[string]int
}

func (c *poolTestConnects) connect(database string) func() (*sql.DB, error) {
	return func() (*sql.DB, error) {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.counts == nil {
			c.counts = make(map[string]int)
		}
		c.counts[database]++
		return sql.OpenDB(poolTestConnector{}), nil
	}
}

func poolTestIsClosed(db *sql.DB) bool { return db.Ping() != nil }

func TestDatabaseConnectionPoolReusesHandleAcrossAcquires(t *testing.T) {
	pool := NewDatabaseConnectionPool(2)
	connects := &poolTestConnects{}
	first, release, err := pool.Acquire("app", connects.connect("app"))
	if err != nil {
		t.Fatal(err)
	}
	release()
	release() // Release is idempotent.
	second, release, err := pool.Acquire("app", connects.connect("app"))
	if err != nil {
		t.Fatal(err)
	}
	release()
	if first != second || connects.counts["app"] != 1 {
		t.Fatalf("handle reused = %t, connects = %d", first == second, connects.counts["app"])
	}
	if poolTestIsClosed(first) {
		t.Fatal("released handle must stay open between collections")
	}
	pool.Close()
	if !poolTestIsClosed(first) {
		t.Fatal("Close must close idle handles")
	}
	if _, _, err := pool.Acquire("app", connects.connect("app")); err == nil {
		t.Fatal("closed pool must refuse new handles")
	}
}

func TestDatabaseConnectionPoolEvictsLeastRecentlyUsed(t *testing.T) {
	pool := NewDatabaseConnectionPool(2)
	connects := &poolTestConnects{}
	handles := map[string]*sql.DB{}
	acquire := func(database string) {
		db, release, err := pool.Acquire(database, connects.connect(database))
		if err != nil {
			t.Fatal(err)
		}
		release()
		handles[database] = db
	}
	acquire("a")
	acquire("b")
	acquire("a") // b is now the least recently used.
	acquire("c")
	if !poolTestIsClosed(handles["b"]) {
		t.Fatal("least recently used handle must be closed on eviction")
	}
	if poolTestIsClosed(handles["a"]) || poolTestIsClosed(handles["c"]) {
		t.Fatal("recently used handles must stay open")
	}
	acquire("b")
	if connects.counts["a"] != 1 || connects.counts["b"] != 2 || connects.counts["c"] != 1 {
		t.Fatalf("connects = %v, want a=1 b=2 c=1", connects.counts)
	}
}

func TestDatabaseConnectionPoolClosesEvictedHandleAfterRelease(t *testing.T) {
	pool := NewDatabaseConnectionPool(1)
	connects := &poolTestConnects{}
	inUse, releaseInUse, err := pool.Acquire("a", connects.connect("a"))
	if err != nil {
		t.Fatal(err)
	}
	_, releaseB, err := pool.Acquire("b", connects.connect("b"))
	if err != nil {
		t.Fatal(err)
	}
	releaseB()
	if poolTestIsClosed(inUse) {
		t.Fatal("evicted handle must stay open while in use")
	}
	releaseInUse()
	if !poolTestIsClosed(inUse) {
		t.Fatal("evicted handle must close on its last release")
	}
}

func TestDatabaseConnectionPoolDoesNotCacheFailures(t *testing.T) {
	pool := NewDatabaseConnectionPool(2)
	attempts := 0
	failing := func() (*sql.DB, error) {
		attempts++
		return nil, errors.New("dial tcp: connection refused")
	}
	for range 2 {
		if db, _, err := pool.Acquire("app", failing); db != nil || err == nil {
			t.Fatal("failed connection must return an error")
		}
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want a retry on every Acquire", attempts)
	}
}

func TestDatabaseConnectionPoolConcurrentAcquireSharesOneHandle(t *testing.T) {
	pool := NewDatabaseConnectionPool(2)
	defer pool.Close()
	connects := &poolTestConnects{}
	const callers = 20
	handles := make([]*sql.DB, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			db, release, err := pool.Acquire("app", connects.connect("app"))
			if err != nil {
				t.Error(err)
				return
			}
			defer release()
			handles[i] = db
		}()
	}
	wg.Wait()
	for _, db := range handles {
		if db != handles[0] || poolTestIsClosed(db) {
			t.Fatal("concurrent callers must share one open handle")
		}
	}
}

func TestDatabaseConnectionPoolNeverEvictsPinnedHandle(t *testing.T) {
	pool := NewDatabaseConnectionPool(2)
	connects := &poolTestConnects{}
	pinned, err := pool.Pin("postgres", connects.connect("postgres"))
	if err != nil {
		t.Fatal(err)
	}
	for _, database := range []string{"a", "b", "c"} {
		_, release, err := pool.Acquire(database, connects.connect(database))
		if err != nil {
			t.Fatal(err)
		}
		release()
	}
	shared, release, err := pool.Acquire("postgres", connects.connect("postgres"))
	if err != nil {
		t.Fatal(err)
	}
	release()
	if shared != pinned || poolTestIsClosed(pinned) || connects.counts["postgres"] != 1 {
		t.Fatalf("pinned handle shared = %t, connects = %d", shared == pinned, connects.counts["postgres"])
	}
	pool.Close()
	if !poolTestIsClosed(pinned) {
		t.Fatal("Close must close pinned handles")
	}
}

func TestDatabaseConnectionPoolFreesIdleHandleBeforeConnecting(t *testing.T) {
	pool := NewDatabaseConnectionPool(1)
	defer pool.Close()
	a, release, err := pool.Acquire("a", (&poolTestConnects{}).connect("a"))
	if err != nil {
		t.Fatal(err)
	}
	release()
	// The server has a single slot: b can connect only after a gives it back.
	_, release, err = pool.Acquire("b", func() (*sql.DB, error) {
		if !poolTestIsClosed(a) {
			return nil, errors.New("pq: sorry, too many clients already")
		}
		return sql.OpenDB(poolTestConnector{}), nil
	})
	if err != nil {
		t.Fatalf("idle handle must be closed before connecting the replacement: %v", err)
	}
	release()
}

func TestDatabaseConnectionPoolKeepsInUseHandleWhenConnectingAtLimit(t *testing.T) {
	pool := NewDatabaseConnectionPool(1)
	defer pool.Close()
	a, releaseA, err := pool.Acquire("a", (&poolTestConnects{}).connect("a"))
	if err != nil {
		t.Fatal(err)
	}
	defer releaseA()
	_, releaseB, err := pool.Acquire("b", func() (*sql.DB, error) {
		if poolTestIsClosed(a) {
			t.Fatal("handle in use must not be closed")
		}
		return sql.OpenDB(poolTestConnector{}), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	releaseB()
}
