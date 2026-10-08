package postgresql

import (
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	logging "github.com/google/logger"
)

func TestPGTableCacheKeepsStatisticsLiveAndRefreshesAfter12Hours(t *testing.T) {
	fake := &pgTableCacheSQL{}
	g := NewDBMetricsConfigGatherer(*logging.Init("pg-table-cache", true, false, io.Discard), &config.Config{}, true, nil)
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	g.now = func() time.Time { return now }
	newPGTableCacheServer(t, g.configuration, fake)
	collect := func() *models.Metrics {
		t.Helper()
		m := &models.Metrics{}
		m.DB.Metrics.Databases = []string{"app"}
		m.DB.Metrics.Status = make(models.MetricGroupValue)
		if err := g.GetMetrics(m); err != nil {
			t.Fatal(err)
		}
		if m.DB.Metrics.TotalTables != 2 || m.DB.Metrics.Engine["BASE TABLE"]["Total Size"] != uint64(100) {
			t.Fatalf("incomplete table metrics: %+v", m.DB.Metrics)
		}
		return m
	}
	first := collect()
	first.DB.Metrics.Engine["BASE TABLE"]["Total Size"] = uint64(999)
	now = now.Add(12*time.Hour - time.Second)
	collect()
	if fake.tableQueryCount() != 1 || fake.statQueryCount() != 2*len(PG_STAT_PER_DB_VIEWS) {
		t.Fatalf("table queries=%d, live statistics=%d", fake.tableQueryCount(), fake.statQueryCount())
	}
	now = now.Add(time.Second)
	fake.setFailure(true, 0)
	collect() // A failed refresh retains the last complete table snapshot.
	fake.setFailure(false, 0)
	collect() // The failed refresh must not extend the TTL.
	if fake.tableQueryCount() != 3 {
		t.Fatalf("table queries after retry = %d, want 3", fake.tableQueryCount())
	}
}

func TestPGTableCacheUsesSharedLargeServerPolicy(t *testing.T) {
	tests := []struct {
		name      string
		threshold int64
		ram       uint64
		rds       bool
		ttl       time.Duration
		wantTTL   time.Duration
	}{
		{"large server custom TTL", 1, 10, false, 3600, time.Hour},
		{"large server seven-day TTL", 1, 10, false, 604800, 7 * 24 * time.Hour},
		{"table count at threshold", 2, 10, false, 3600, 12 * time.Hour},
		{"size at RAM threshold", 1, 25, false, 3600, 12 * time.Hour},
		{"missing physical RAM", 1, 0, false, 3600, 12 * time.Hour},
		{"RDS RAM uses KiB", 1, 1, true, 3600, 12 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &pgTableCacheSQL{}
			cfg := &config.Config{TableSizeCacheTableThreshold: tt.threshold, TableSizeCacheRAMMultiplier: 4, TableSizeCacheTTL: tt.ttl}
			g := NewDBMetricsConfigGatherer(*logging.Init("pg-cache-policy", true, false, io.Discard), cfg, true, nil)
			base := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
			now := base
			g.now = func() time.Time { return now }
			newPGTableCacheServer(t, g.configuration, fake)
			collect := func() {
				t.Helper()
				m := &models.Metrics{}
				m.System.Info = models.MetricGroupValue{"PhysicalMemory": models.MetricGroupValue{"total": tt.ram}}
				if tt.rds {
					m.System.Info["Host"] = models.MetricGroupValue{"InstanceType": "aws/rds"}
				}
				m.DB.Metrics.Databases = []string{"app"}
				m.DB.Metrics.Status = make(models.MetricGroupValue)
				if err := g.GetMetrics(m); err != nil {
					t.Fatal(err)
				}
			}
			collect()
			now = base.Add(tt.wantTTL - time.Second)
			collect()
			if fake.tableQueryCount() != 1 {
				t.Fatalf("queries before expiry = %d, want 1", fake.tableQueryCount())
			}
			now = base.Add(tt.wantTTL)
			collect()
			if fake.tableQueryCount() != 2 || fake.statQueryCount() != 3*len(PG_STAT_PER_DB_VIEWS) {
				t.Fatalf("queries at expiry = %d, live statistics = %d", fake.tableQueryCount(), fake.statQueryCount())
			}
		})
	}
}

func TestPGTableCacheRefreshesWhenRAMChangesCategory(t *testing.T) {
	fake := &pgTableCacheSQL{}
	cfg := &config.Config{TableSizeCacheTableThreshold: 1, TableSizeCacheRAMMultiplier: 4, TableSizeCacheTTL: 604800}
	g := NewDBMetricsConfigGatherer(*logging.Init("pg-cache-category", true, false, io.Discard), cfg, true, nil)
	now := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	g.now = func() time.Time { return now }
	newPGTableCacheServer(t, g.configuration, fake)
	collect := func(ram uint64) {
		t.Helper()
		m := &models.Metrics{}
		m.System.Info = models.MetricGroupValue{"PhysicalMemory": models.MetricGroupValue{"total": ram}}
		m.DB.Metrics.Databases = []string{"app"}
		m.DB.Metrics.Status = make(models.MetricGroupValue)
		if err := g.GetMetrics(m); err != nil {
			t.Fatal(err)
		}
		if m.DB.Metrics.TotalTables != 2 || m.DB.Metrics.Engine["BASE TABLE"]["Total Size"] != uint64(100) {
			t.Fatalf("incomplete snapshot: %+v", m.DB.Metrics)
		}
	}
	collect(10) // Large server, seven-day TTL.
	now = now.Add(time.Minute)
	collect(25) // Same snapshot count, larger RAM: force ordinary-server refresh.
	if fake.tableQueryCount() != 2 {
		t.Fatalf("queries after category change = %d, want 2", fake.tableQueryCount())
	}
	now = now.Add(time.Minute)
	fake.setFailure(true, 0)
	collect(10) // The failed transition retains the complete snapshot.
	fake.setFailure(false, 0)
	collect(10) // The category change must still trigger a retry before expiry.
	if fake.tableQueryCount() != 4 || fake.statQueryCount() != 4*len(PG_STAT_PER_DB_VIEWS) {
		t.Fatalf("queries after retry = %d, live statistics = %d", fake.tableQueryCount(), fake.statQueryCount())
	}
}

func TestPGTableCacheDoesNotCacheInitialFailure(t *testing.T) {
	fake := &pgTableCacheSQL{failTables: true}
	g := NewDBMetricsConfigGatherer(*logging.Init("pg-cache-initial-failure", true, false, io.Discard), &config.Config{}, true, nil)
	newPGTableCacheServer(t, g.configuration, fake)
	collect := func() (*models.Metrics, error) {
		m := &models.Metrics{}
		m.DB.Metrics.Databases = []string{"app"}
		m.DB.Metrics.Status = make(models.MetricGroupValue)
		return m, g.GetMetrics(m)
	}
	if m, err := collect(); err == nil {
		t.Fatal("initial table-size failure must be returned")
	} else if m.DB.Metrics.Engine != nil {
		t.Fatal("initial failure published incomplete size aggregates")
	}
	minute := &models.Metrics{}
	minute.DB.Metrics.Databases = []string{"app"}
	minute.DB.Metrics.Status = make(models.MetricGroupValue)
	if err := NewDBMetricsConfigGatherer(g.logger, g.configuration, false, g.tableSizeCache).GetMetrics(minute); err != nil {
		t.Fatal(err)
	}
	if fake.tableQueryCount() != 1 || minute.DB.Metrics.Engine != nil {
		t.Fatal("minute retried or cached an initial failure")
	}
	fake.setFailure(false, 0)
	m, err := collect()
	if err != nil {
		t.Fatal(err)
	}
	if m.DB.Metrics.TotalTables != 2 {
		t.Fatalf("successful retry has %d tables", m.DB.Metrics.TotalTables)
	}
	if fake.tableQueryCount() != 2 {
		t.Fatalf("table queries = %d, want 2", fake.tableQueryCount())
	}
}

func TestPGTableCacheDoesNotReplaceSnapshotWithPartialRefresh(t *testing.T) {
	fake := &pgTableCacheSQL{}
	g := NewDBMetricsConfigGatherer(*logging.Init("pg-cache-partial-refresh", true, false, io.Discard), &config.Config{}, true, nil)
	now := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	g.now = func() time.Time { return now }
	newPGTableCacheServer(t, g.configuration, fake)
	collect := func() *models.Metrics {
		m := &models.Metrics{}
		m.DB.Metrics.Databases = []string{"app", "archive"}
		m.DB.Metrics.Status = make(models.MetricGroupValue)
		if err := g.GetMetrics(m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	collect()
	now = now.Add(12 * time.Hour)
	fake.setFailure(false, 4) // Second database's size query, after a partial refresh.
	if m := collect(); m.DB.Metrics.TotalTables != 4 || m.DB.Metrics.Engine["BASE TABLE"]["Total Size"] != uint64(200) {
		t.Fatalf("partial refresh replaced successful snapshot: %+v", m.DB.Metrics)
	}
	collect()
	if fake.tableQueryCount() != 6 {
		t.Fatalf("table queries after retry = %d, want 6", fake.tableQueryCount())
	}
}

func TestPGTableCacheConcurrentReportsRefreshOnlyOnce(t *testing.T) {
	fake := &pgTableCacheSQL{}
	g := NewDBMetricsConfigGatherer(*logging.Init("pg-cache-concurrent", true, false, io.Discard), &config.Config{}, true, nil)
	newPGTableCacheServer(t, g.configuration, fake)
	const callers = 10
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m := &models.Metrics{}
			m.DB.Metrics.Databases = []string{"app"}
			m.DB.Metrics.Status = make(models.MetricGroupValue)
			if err := g.GetMetrics(m); err != nil {
				t.Error(err)
			}
			if m.DB.Metrics.TotalTables != 2 {
				t.Errorf("tables = %d", m.DB.Metrics.TotalTables)
			}
		}()
	}
	wg.Wait()
	if fake.tableQueryCount() != 1 || fake.statQueryCount() != callers*len(PG_STAT_PER_DB_VIEWS) {
		t.Fatalf("table queries=%d, live statistics=%d", fake.tableQueryCount(), fake.statQueryCount())
	}
}

type pgTableCacheSQL struct {
	mu                        sync.Mutex
	tableQueries, statQueries int
	failTables                bool
	failTableQuery            int
	failStatisticsDatabase    string
}

func (f *pgTableCacheSQL) tableQueryCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.tableQueries
}

func (f *pgTableCacheSQL) statQueryCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.statQueries
}

func (f *pgTableCacheSQL) setFailure(all bool, query int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failTables = all
	f.failTableQuery = query
}

func (f *pgTableCacheSQL) query(database, q string) (driver.Rows, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.Contains(q, "information_schema.tables") {
		f.tableQueries++
		if f.failTables || f.tableQueries == f.failTableQuery {
			return nil, errors.New("table query unavailable")
		}
		if strings.Contains(q, "SELECT COUNT(*)") {
			return nil, errors.New("unexpected standalone table count query")
		}
		return &pgTableCacheRows{columns: []string{"table_type", "table_count", "total_size"}, values: []driver.Value{"BASE TABLE", int64(2), int64(100)}}, nil
	}
	if database == f.failStatisticsDatabase {
		return nil, errors.New("pg_stat_user_tables unavailable")
	}
	f.statQueries++
	return &pgTableCacheRows{columns: []string{"value"}, values: []driver.Value{int64(f.statQueries)}}, nil
}

type pgTableCacheRows struct {
	columns []string
	values  []driver.Value
	done    bool
}

func (r *pgTableCacheRows) Columns() []string { return r.columns }
func (r *pgTableCacheRows) Close() error      { return nil }
func (r *pgTableCacheRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	copy(dest, r.values)
	return nil
}

func TestPGMinuteTableSizesNeverRefreshMissingExpiredOrFailedCache(t *testing.T) {
	fake := &pgTableCacheSQL{}
	g := NewDBMetricsConfigGatherer(*logging.Init("pg-minute-cache", false, false, io.Discard), &config.Config{}, true, nil)
	newPGTableCacheServer(t, g.configuration, fake)
	now := time.Now()
	g.now = func() time.Time { return now }
	collect := func(full bool, want bool) {
		t.Helper()
		m := &models.Metrics{}
		m.DB.Metrics.Databases = []string{"app"}
		m.DB.Metrics.Status = make(models.MetricGroupValue)
		var err error
		if full {
			err = g.GetMetrics(m)
		} else {
			err = NewDBMetricsConfigGatherer(g.logger, g.configuration, false, g.tableSizeCache).GetMetrics(m)
		}
		if err != nil {
			t.Fatal(err)
		}
		if want && m.DB.Metrics.TotalTables != 2 {
			t.Fatalf("cached count=%d", m.DB.Metrics.TotalTables)
		}
		if !want && m.DB.Metrics.Engine != nil {
			t.Fatalf("empty cache fabricated sizes: %v", m.DB.Metrics.Engine)
		}
	}
	collect(false, false)
	if fake.tableQueryCount() != 0 {
		t.Fatal("minute collected sizes with empty cache")
	}
	collect(true, true)
	now = now.Add(24 * time.Hour)
	collect(false, true)
	if fake.tableQueryCount() != 1 {
		t.Fatal("minute refreshed expired cache")
	}
	fake.setFailure(true, 0)
	collect(true, true)
	calls := fake.tableQueryCount()
	collect(false, true)
	if fake.tableQueryCount() != calls {
		t.Fatal("minute retried failed refresh")
	}
	fake.setFailure(false, 0)
	now = now.Add(12 * time.Hour)
	collect(true, true)
	if fake.tableQueryCount() != calls+1 {
		t.Fatal("full collection did not retry")
	}
}
