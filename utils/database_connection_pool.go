package utils

import (
	"container/list"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Releem/mysqlconfigurer/config"
)

// pooledDatabaseMaxIdleTime closes a handle's idle server connection when its
// database is not used for a while; the handle reconnects on the next query.
const pooledDatabaseMaxIdleTime = 10 * time.Minute

var errDatabaseConnectionPoolClosed = errors.New("database connection pool is closed")

// DatabaseConnectionPool keeps one *sql.DB per database name across collections
// and closes the least recently used handle when more than max databases are
// open. Pinned handles are never evicted. A handle evicted while in use is
// closed by its last Release. Failed
// connections are not cached, so the next Acquire retries them. It is safe for
// concurrent use.
type DatabaseConnectionPool struct {
	mu        sync.Mutex
	max       int
	databases map[string]*list.Element
	order     *list.List // front is the most recently used database
	closed    bool
}

type pooledDatabase struct {
	database string
	db       *sql.DB
	refs     int
	evicted  bool
	pinned   bool
}

// NewDatabaseConnectionPool creates a pool that keeps at most max databases
// open; a non-positive max uses config.DefaultPgMaxDatabaseConnections.
func NewDatabaseConnectionPool(max int) *DatabaseConnectionPool {
	if max < 1 {
		max = config.DefaultPgMaxDatabaseConnections
	}
	return &DatabaseConnectionPool{max: max, databases: make(map[string]*list.Element), order: list.New()}
}

// Acquire returns the open handle for database, calling connect only when the
// pool has none. The caller must call release once it has finished using db.
func (pool *DatabaseConnectionPool) Acquire(database string, connect func() (*sql.DB, error)) (db *sql.DB, release func(), err error) {
	pool.mu.Lock()
	if pool.closed {
		pool.mu.Unlock()
		return nil, nil, errDatabaseConnectionPoolClosed
	}
	if entry := pool.use(database); entry != nil {
		pool.mu.Unlock()
		return entry.db, pool.releaser(entry), nil
	}
	// Free a server connection before opening the replacement: a server with no
	// spare slots would otherwise reject every new database while idle handles
	// keep theirs.
	if pool.order.Len() >= pool.max {
		pool.evictIdle()
	}
	pool.mu.Unlock()

	// Connect outside the lock so a slow database does not block the others.
	db, err = connect()
	if db == nil {
		if err == nil {
			err = fmt.Errorf("connect to database %q: nil database handle", database)
		}
		return nil, nil, err
	}
	db.SetMaxIdleConns(1)
	db.SetConnMaxIdleTime(pooledDatabaseMaxIdleTime)

	pool.mu.Lock()
	defer pool.mu.Unlock()
	if pool.closed {
		_ = db.Close()
		return nil, nil, errDatabaseConnectionPoolClosed
	}
	if entry := pool.use(database); entry != nil {
		// Another caller connected while this one was connecting.
		_ = db.Close()
		return entry.db, pool.releaser(entry), nil
	}
	entry := &pooledDatabase{database: database, db: db, refs: 1}
	pool.databases[database] = pool.order.PushFront(entry)
	pool.evictOverLimit()
	return db, pool.releaser(entry), nil
}

// Pin opens database like Acquire and keeps its handle until Close, so the
// handle can be shared outside the pool, for example as the agent's main
// connection. Pinned handles count toward max but are never evicted.
func (pool *DatabaseConnectionPool) Pin(database string, connect func() (*sql.DB, error)) (*sql.DB, error) {
	db, release, err := pool.Acquire(database, connect)
	if err != nil {
		return nil, err
	}
	pool.mu.Lock()
	if element := pool.databases[database]; element != nil {
		element.Value.(*pooledDatabase).pinned = true
	}
	pool.mu.Unlock()
	release()
	return db, nil
}

// Close closes every handle that is not in use; handles in use close on release.
func (pool *DatabaseConnectionPool) Close() {
	if pool == nil {
		return
	}
	pool.mu.Lock()
	defer pool.mu.Unlock()
	pool.closed = true
	for pool.order.Len() > 0 {
		pool.evict(pool.order.Back())
	}
}

// use marks database as most recently used and takes a reference to it.
func (pool *DatabaseConnectionPool) use(database string) *pooledDatabase {
	element := pool.databases[database]
	if element == nil {
		return nil
	}
	pool.order.MoveToFront(element)
	entry := element.Value.(*pooledDatabase)
	entry.refs++
	return entry
}

// evictOverLimit closes the least recently used unpinned handles while more
// than max databases are open.
func (pool *DatabaseConnectionPool) evictOverLimit() {
	for element := pool.order.Back(); element != nil && pool.order.Len() > pool.max; {
		previous := element.Prev()
		if !element.Value.(*pooledDatabase).pinned {
			pool.evict(element)
		}
		element = previous
	}
}

// evictIdle closes the least recently used handle that is neither pinned nor
// in use. A handle in use would keep its server connection until release.
func (pool *DatabaseConnectionPool) evictIdle() {
	for element := pool.order.Back(); element != nil; element = element.Prev() {
		if entry := element.Value.(*pooledDatabase); !entry.pinned && entry.refs == 0 {
			pool.evict(element)
			return
		}
	}
}

func (pool *DatabaseConnectionPool) evict(element *list.Element) {
	entry := element.Value.(*pooledDatabase)
	pool.order.Remove(element)
	delete(pool.databases, entry.database)
	entry.evicted = true
	if entry.refs == 0 {
		_ = entry.db.Close()
	}
}

func (pool *DatabaseConnectionPool) releaser(entry *pooledDatabase) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			pool.mu.Lock()
			defer pool.mu.Unlock()
			entry.refs--
			if entry.evicted && entry.refs == 0 {
				_ = entry.db.Close()
			}
		})
	}
}
