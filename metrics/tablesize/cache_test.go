package tablesize

import (
	"errors"
	"math"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Releem/mysqlconfigurer/models"
	snapshotcache "github.com/Releem/mysqlconfigurer/utils/cache"
)

func TestTableSizeCacheThresholds(t *testing.T) {
	const gib = uint64(1024 * 1024 * 1024)
	baseTime := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		totalTables   uint64
		effectiveRAM  uint64
		tableLimit    int64
		ramMultiplier int64
		engine        map[string]models.MetricGroupValue
		wantTTL       time.Duration
	}{
		{
			name:          "table count equal to threshold uses ordinary cache",
			totalTables:   1000,
			effectiveRAM:  gib,
			tableLimit:    1000,
			ramMultiplier: 4,
			engine: tableSizeTestEngine(
				4*gib+1,
				128,
			),
			wantTTL: 11*time.Hour + 55*time.Minute,
		},
		{
			name:          "total size equal to RAM multiple uses ordinary cache",
			totalTables:   1001,
			effectiveRAM:  gib,
			tableLimit:    1000,
			ramMultiplier: 4,
			engine: tableSizeTestEngine(
				3*gib,
				gib,
			),
			wantTTL: 11*time.Hour + 55*time.Minute,
		},
		{
			name:          "both values greater than thresholds cache",
			totalTables:   1001,
			effectiveRAM:  gib,
			tableLimit:    1000,
			ramMultiplier: 4,
			engine: tableSizeTestEngine(
				3*gib,
				gib+1,
			),
			wantTTL: time.Hour,
		},
		{
			name:          "table threshold failing uses ordinary cache",
			totalTables:   999,
			effectiveRAM:  gib,
			tableLimit:    1000,
			ramMultiplier: 4,
			engine: tableSizeTestEngine(
				5*gib,
				128,
			),
			wantTTL: 11*time.Hour + 55*time.Minute,
		},
		{
			name:          "RAM threshold failing uses ordinary cache",
			totalTables:   1001,
			effectiveRAM:  gib,
			tableLimit:    1000,
			ramMultiplier: 4,
			engine: tableSizeTestEngine(
				3*gib,
				gib-1,
			),
			wantTTL: 11*time.Hour + 55*time.Minute,
		},
		{
			name:          "total size addition saturates",
			totalTables:   1001,
			effectiveRAM:  math.MaxUint64 / 2,
			tableLimit:    1000,
			ramMultiplier: 1,
			engine: tableSizeTestEngine(
				math.MaxUint64,
				1,
			),
			wantTTL: time.Hour,
		},
		{
			name:          "RAM multiplication overflow uses ordinary cache",
			totalTables:   1001,
			effectiveRAM:  math.MaxUint64,
			tableLimit:    1000,
			ramMultiplier: 2,
			engine: tableSizeTestEngine(
				math.MaxUint64,
				0,
			),
			wantTTL: 11*time.Hour + 55*time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := newTableSizeCache()
			calls := 0
			refresh := func() (map[string]models.MetricGroupValue, error) {
				calls++
				return tt.engine, nil
			}
			input := Input{
				TotalTables:    tableCount(tt.totalTables),
				EffectiveRAM:   tt.effectiveRAM,
				TableThreshold: tt.tableLimit,
				RAMMultiplier:  tt.ramMultiplier,
				TTL:            time.Hour,
				Now:            baseTime,
			}

			if _, err := cache.GetOrRefresh(input, refresh); err != nil {
				t.Fatalf("first getOrRefresh() error = %v", err)
			}
			if _, err := cache.GetOrRefresh(input, refresh); err != nil {
				t.Fatalf("second getOrRefresh() error = %v", err)
			}

			input.Now = baseTime.Add(tt.wantTTL - time.Second)
			if _, err := cache.GetOrRefresh(input, refresh); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("refresh calls before expiry = %d, want 1", calls)
			}
			input.Now = baseTime.Add(tt.wantTTL)
			if _, err := cache.GetOrRefresh(input, refresh); err != nil {
				t.Fatal(err)
			}
			if calls != 2 {
				t.Fatalf("refresh calls at expiry = %d, want 2", calls)
			}
		})
	}
}

func TestTableSizeCacheZeroEffectiveRAMUsesOrdinaryTTL(t *testing.T) {
	const gib = uint64(1024 * 1024 * 1024)
	cache := newTableSizeCache()
	calls := 0
	refresh := func() (map[string]models.MetricGroupValue, error) {
		calls++
		return tableSizeTestEngine(5*gib, 1024), nil
	}
	input := Input{
		TotalTables:    tableCount(1001),
		EffectiveRAM:   0,
		TableThreshold: 1000,
		RAMMultiplier:  4,
		TTL:            time.Hour,
		Now:            time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC),
	}

	if _, err := cache.GetOrRefresh(input, refresh); err != nil {
		t.Fatalf("first getOrRefresh() error = %v", err)
	}
	if _, err := cache.GetOrRefresh(input, refresh); err != nil {
		t.Fatalf("second getOrRefresh() error = %v", err)
	}

	input.Now = input.Now.Add(11*time.Hour + 55*time.Minute - time.Second)
	if _, err := cache.GetOrRefresh(input, refresh); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("refresh calls = %d, want 1", calls)
	}
	input.Now = input.Now.Add(time.Second)
	if _, err := cache.GetOrRefresh(input, refresh); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("refresh calls at 11h55m = %d, want 2", calls)
	}
}

func TestTableSizeCacheIneligibleSuccessNotReusedAfterInputsBecomeEligible(t *testing.T) {
	const gib = uint64(1024 * 1024 * 1024)
	cache := newTableSizeCache()
	calls := 0
	refresh := func() (map[string]models.MetricGroupValue, error) {
		calls++
		if calls == 1 {
			return tableSizeTestEngine(5*gib, 1024), nil
		}
		return tableSizeTestEngine(6*gib, 2048), nil
	}
	input := Input{
		TotalTables:    tableCount(1000),
		EffectiveRAM:   gib,
		TableThreshold: 1000,
		RAMMultiplier:  4,
		TTL:            time.Hour,
		Now:            time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC),
	}

	if _, err := cache.GetOrRefresh(input, refresh); err != nil {
		t.Fatalf("ineligible getOrRefresh() error = %v", err)
	}
	input.TotalTables = tableCount(1001)
	eligible, err := cache.GetOrRefresh(input, refresh)
	if err != nil {
		t.Fatalf("eligible getOrRefresh() error = %v", err)
	}

	if calls != 2 {
		t.Fatalf("refresh calls = %d, want 2", calls)
	}
	if got := eligible.Value["InnoDB"]["Total Size"]; got != 6*gib {
		t.Fatalf("eligible InnoDB Total Size = %v, want %d", got, 6*gib)
	}
}

func TestTableSizeCacheIneligibleSuccessUsedAsStaleFallback(t *testing.T) {
	const gib = uint64(1024 * 1024 * 1024)
	refreshFailure := errors.New("second table size query failed")
	cache := newTableSizeCache()
	calls := 0
	refresh := func() (map[string]models.MetricGroupValue, error) {
		calls++
		if calls == 1 {
			return tableSizeTestEngine(5*gib, 1024), nil
		}
		return nil, refreshFailure
	}
	input := Input{
		TotalTables:    tableCount(1000),
		EffectiveRAM:   gib,
		TableThreshold: 1000,
		RAMMultiplier:  4,
		TTL:            time.Hour,
		Now:            time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC),
	}

	if _, err := cache.GetOrRefresh(input, refresh); err != nil {
		t.Fatalf("ineligible getOrRefresh() error = %v", err)
	}
	input.Now = input.Now.Add(11*time.Hour + 55*time.Minute)
	result, err := cache.GetOrRefresh(input, refresh)

	if err != nil {
		t.Fatalf("second getOrRefresh() error = %v, want stale fallback", err)
	}
	if !result.UsedStale || !errors.Is(result.RefreshErr, refreshFailure) {
		t.Fatalf("second result = %#v, want stale result with refresh error", result)
	}
	if got := result.Value["InnoDB"]["Total Size"]; got != 5*gib {
		t.Fatalf("stale InnoDB Total Size = %v, want %d", got, 5*gib)
	}
	if calls != 2 {
		t.Fatalf("refresh calls = %d, want 2", calls)
	}
}

func TestTableSizeCacheEligibleSuccessThenIneligibleSuccessForcesNextRefresh(t *testing.T) {
	const gib = uint64(1024 * 1024 * 1024)
	cache := newTableSizeCache()
	calls := 0
	refresh := func() (map[string]models.MetricGroupValue, error) {
		calls++
		switch calls {
		case 1:
			return tableSizeTestEngine(5*gib, 1), nil
		case 2:
			return tableSizeTestEngine(6*gib, 2), nil
		default:
			return tableSizeTestEngine(7*gib, 3), nil
		}
	}
	input := Input{
		TotalTables:    tableCount(1001),
		EffectiveRAM:   gib,
		TableThreshold: 1000,
		RAMMultiplier:  4,
		TTL:            time.Hour,
		Now:            time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC),
	}

	if _, err := cache.GetOrRefresh(input, refresh); err != nil {
		t.Fatalf("eligible getOrRefresh() error = %v", err)
	}
	input.TotalTables = tableCount(1000)
	ineligible, err := cache.GetOrRefresh(input, refresh)
	if err != nil {
		t.Fatalf("ineligible getOrRefresh() error = %v", err)
	}
	if got := ineligible.Value["InnoDB"]["Total Size"]; got != 6*gib {
		t.Fatalf("ineligible InnoDB Total Size = %v, want %d", got, 6*gib)
	}

	input.TotalTables = tableCount(1001)
	next, err := cache.GetOrRefresh(input, refresh)
	if err != nil {
		t.Fatalf("next eligible getOrRefresh() error = %v", err)
	}
	if calls != 3 {
		t.Fatalf("refresh calls = %d, want 3", calls)
	}
	if got := next.Value["InnoDB"]["Total Size"]; got != 7*gib {
		t.Fatalf("next eligible InnoDB Total Size = %v, want %d", got, 7*gib)
	}
}

func TestTableSizeCacheLatestIneligibleSuccessReplacesStaleFallback(t *testing.T) {
	const gib = uint64(1024 * 1024 * 1024)
	refreshFailure := errors.New("next table size query failed")
	cache := newTableSizeCache()
	calls := 0
	refresh := func() (map[string]models.MetricGroupValue, error) {
		calls++
		switch calls {
		case 1:
			return tableSizeTestEngine(5*gib, 1), nil
		case 2:
			return tableSizeTestEngine(6*gib, 2), nil
		default:
			return nil, refreshFailure
		}
	}
	input := Input{
		TotalTables:    tableCount(1001),
		EffectiveRAM:   gib,
		TableThreshold: 1000,
		RAMMultiplier:  4,
		TTL:            time.Hour,
		Now:            time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC),
	}

	if _, err := cache.GetOrRefresh(input, refresh); err != nil {
		t.Fatalf("eligible getOrRefresh() error = %v", err)
	}
	input.TotalTables = tableCount(1000)
	if _, err := cache.GetOrRefresh(input, refresh); err != nil {
		t.Fatalf("ineligible getOrRefresh() error = %v", err)
	}

	input.TotalTables = tableCount(1001)
	stale, err := cache.GetOrRefresh(input, refresh)
	if err != nil {
		t.Fatalf("stale getOrRefresh() error = %v, want nil", err)
	}
	if calls != 3 {
		t.Fatalf("refresh calls = %d, want 3", calls)
	}
	if !stale.UsedStale {
		t.Fatal("usedStale = false, want true")
	}
	if !errors.Is(stale.RefreshErr, refreshFailure) {
		t.Fatalf("refreshErr = %v, want %v", stale.RefreshErr, refreshFailure)
	}
	if got := stale.Value["InnoDB"]["Total Size"]; got != 6*gib {
		t.Fatalf("stale InnoDB Total Size = %v, want latest successful %d", got, 6*gib)
	}
}

func TestTableSizeCacheClonesSnapshots(t *testing.T) {
	const gib = uint64(1024 * 1024 * 1024)
	now := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
	refreshed := map[string]models.MetricGroupValue{
		"InnoDB": {
			"Enabled":      "YES",
			"Table Number": uint64(1200),
			"Total Size":   4 * gib,
			"Data Size":    3 * gib,
			"Index Size":   gib,
		},
		"MyISAM": {
			"Enabled":      "YES",
			"Table Number": uint64(2),
			"Total Size":   uint64(4096),
			"Data Size":    uint64(3072),
			"Index Size":   uint64(1024),
		},
	}
	cache := newTableSizeCache()
	calls := 0
	refresh := func() (map[string]models.MetricGroupValue, error) {
		calls++
		return refreshed, nil
	}
	input := Input{
		TotalTables:    tableCount(1202),
		EffectiveRAM:   gib,
		TableThreshold: 1000,
		RAMMultiplier:  4,
		TTL:            time.Hour,
		Now:            now,
	}

	first, err := cache.GetOrRefresh(input, refresh)
	if err != nil {
		t.Fatalf("first getOrRefresh() error = %v", err)
	}
	delete(first.Value, "MyISAM")
	first.Value["InnoDB"]["Index Size"] = uint64(7)
	refreshed["MyISAM"]["Index Size"] = uint64(9)

	second, err := cache.GetOrRefresh(input, refresh)
	if err != nil {
		t.Fatalf("second getOrRefresh() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("refresh calls = %d, want 1", calls)
	}
	if got := second.Value["InnoDB"]["Index Size"]; got != gib {
		t.Fatalf("cached InnoDB Index Size = %v, want %d", got, gib)
	}
	if got := second.Value["MyISAM"]["Index Size"]; got != uint64(1024) {
		t.Fatalf("cached MyISAM Index Size = %v, want 1024", got)
	}
	if got := second.Value["InnoDB"]["Data Size"]; got != 3*gib {
		t.Fatalf("cached InnoDB Data Size = %v, want %d", got, 3*gib)
	}
	if got := second.Value["MyISAM"]["Data Size"]; got != uint64(3072) {
		t.Fatalf("cached MyISAM Data Size = %v, want 3072", got)
	}

	second.Value["MyISAM"]["Data Size"] = uint64(11)
	third, err := cache.GetOrRefresh(input, refresh)
	if err != nil {
		t.Fatalf("third getOrRefresh() error = %v", err)
	}
	if got := third.Value["MyISAM"]["Data Size"]; got != uint64(3072) {
		t.Fatalf("independent MyISAM Data Size = %v, want 3072", got)
	}
}

func tableSizeTestEngine(innodbTotal, myisamTotal uint64) map[string]models.MetricGroupValue {
	return map[string]models.MetricGroupValue{
		"InnoDB": {
			"Enabled":      "YES",
			"Table Number": uint64(1000),
			"Total Size":   innodbTotal,
			"Data Size":    innodbTotal,
			"Index Size":   uint64(0),
		},
		"MyISAM": {
			"Enabled":      "YES",
			"Table Number": uint64(1),
			"Total Size":   myisamTotal,
			"Data Size":    uint64(0),
			"Index Size":   myisamTotal,
		},
	}
}

func TestTableSizeCacheTTLBoundary(t *testing.T) {
	const gib = uint64(1024 * 1024 * 1024)
	for _, tc := range []struct {
		name      string
		elapsed   time.Duration
		wantCalls int
	}{
		{"one second before expiry", 24*time.Hour - time.Second, 1},
		{"exactly at expiry", 24 * time.Hour, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			collectedAt := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
			cache := newTableSizeCache()
			calls := 0
			refresh := func() (map[string]models.MetricGroupValue, error) {
				calls++
				return tableSizeTestEngine(4*gib, 1), nil
			}
			input := Input{TotalTables: tableCount(1001), EffectiveRAM: gib, TableThreshold: 1000, RAMMultiplier: 4, TTL: 24 * time.Hour, Now: collectedAt}
			if _, err := cache.GetOrRefresh(input, refresh); err != nil {
				t.Fatal(err)
			}
			input.Now = collectedAt.Add(tc.elapsed)
			if _, err := cache.GetOrRefresh(input, refresh); err != nil {
				t.Fatal(err)
			}
			if calls != tc.wantCalls {
				t.Fatalf("GetOrRefresh(elapsed=%s) refresh calls = %d, want %d", tc.elapsed, calls, tc.wantCalls)
			}
		})
	}
}

func TestTableSizeCacheTTLSuccessfulRefreshReplacesSnapshotAndTimestamp(t *testing.T) {
	const gib = uint64(1024 * 1024 * 1024)
	firstTime := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
	secondTime := firstTime.Add(24 * time.Hour)
	cache := newTableSizeCache()
	calls := 0
	refresh := func() (map[string]models.MetricGroupValue, error) {
		calls++
		if calls == 1 {
			return tableSizeTestEngine(4*gib, 1), nil
		}
		return tableSizeTestEngine(5*gib, 2), nil
	}
	input := Input{
		TotalTables:    tableCount(1001),
		EffectiveRAM:   gib,
		TableThreshold: 1000,
		RAMMultiplier:  4,
		TTL:            24 * time.Hour,
		Now:            firstTime,
	}

	if _, err := cache.GetOrRefresh(input, refresh); err != nil {
		t.Fatalf("first getOrRefresh() error = %v", err)
	}
	input.Now = secondTime
	second, err := cache.GetOrRefresh(input, refresh)
	if err != nil {
		t.Fatalf("second getOrRefresh() error = %v", err)
	}
	if got := second.Value["InnoDB"]["Total Size"]; got != 5*gib {
		t.Fatalf("refreshed InnoDB Total Size = %v, want %d", got, 5*gib)
	}

	input.Now = secondTime.Add(24*time.Hour - time.Second)
	third, err := cache.GetOrRefresh(input, refresh)
	if err != nil {
		t.Fatalf("third getOrRefresh() error = %v", err)
	}
	if got := third.Value["MyISAM"]["Total Size"]; got != uint64(2) {
		t.Fatalf("cached MyISAM Total Size = %v, want 2", got)
	}
	if calls != 2 {
		t.Fatalf("refresh calls = %d, want 2", calls)
	}
}

func TestTableSizeCacheStaleFallbackReturnsSnapshotAndRefreshError(t *testing.T) {
	const gib = uint64(1024 * 1024 * 1024)
	refreshFailure := errors.New("table size query failed")
	collectedAt := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
	cache := newTableSizeCache()
	calls := 0
	refresh := func() (map[string]models.MetricGroupValue, error) {
		calls++
		if calls == 1 {
			return tableSizeTestEngine(4*gib, 1024), nil
		}
		return nil, refreshFailure
	}
	input := Input{
		TotalTables:    tableCount(1001),
		EffectiveRAM:   gib,
		TableThreshold: 1000,
		RAMMultiplier:  4,
		TTL:            time.Hour,
		Now:            collectedAt,
	}

	if _, err := cache.GetOrRefresh(input, refresh); err != nil {
		t.Fatalf("first getOrRefresh() error = %v", err)
	}
	input.Now = collectedAt.Add(time.Hour)
	stale, err := cache.GetOrRefresh(input, refresh)

	if err != nil {
		t.Fatalf("stale getOrRefresh() error = %v, want nil", err)
	}
	if !stale.UsedStale {
		t.Fatal("usedStale = false, want true")
	}
	if !errors.Is(stale.RefreshErr, refreshFailure) {
		t.Fatalf("refreshErr = %v, want %v", stale.RefreshErr, refreshFailure)
	}
	if got := stale.Value["InnoDB"]["Total Size"]; got != 4*gib {
		t.Fatalf("stale InnoDB Total Size = %v, want %d", got, 4*gib)
	}
	if got := stale.Value["MyISAM"]["Index Size"]; got != uint64(1024) {
		t.Fatalf("stale MyISAM Index Size = %v, want 1024", got)
	}
	if calls != 2 {
		t.Fatalf("refresh calls = %d, want 2", calls)
	}
}

func TestTableSizeCacheFirstErrorReturnsOriginalError(t *testing.T) {
	refreshFailure := errors.New("initial table size query failed")
	cache := newTableSizeCache()

	result, err := cache.GetOrRefresh(
		Input{
			TotalTables:    tableCount(1001),
			EffectiveRAM:   1024,
			TableThreshold: 1000,
			RAMMultiplier:  4,
			TTL:            time.Hour,
			Now:            time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC),
		},
		func() (map[string]models.MetricGroupValue, error) {
			return nil, refreshFailure
		},
	)

	if !errors.Is(err, refreshFailure) {
		t.Fatalf("getOrRefresh() error = %v, want %v", err, refreshFailure)
	}
	if result.Value != nil {
		t.Fatalf("result.Value = %v, want nil", result.Value)
	}
}

func TestTableSizeCacheConcurrentCallersShareRefreshAndReceiveIndependentSnapshots(t *testing.T) {
	const (
		callers = 20
		gib     = uint64(1024 * 1024 * 1024)
	)
	cache := newTableSizeCache()
	input := Input{
		TotalTables:    tableCount(1001),
		EffectiveRAM:   gib,
		TableThreshold: 1000,
		RAMMultiplier:  4,
		TTL:            time.Hour,
		Now:            time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC),
	}
	start := make(chan struct{})
	refreshStarted := make(chan struct{}, 1)
	releaseRefresh := make(chan struct{})
	var calls int32
	refresh := func() (map[string]models.MetricGroupValue, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			refreshStarted <- struct{}{}
		}
		<-releaseRefresh
		return tableSizeTestEngine(4*gib, 1024), nil
	}
	type response struct {
		result snapshotcache.Result[map[string]models.MetricGroupValue]
		err    error
	}
	responses := make(chan response, callers)

	for i := 0; i < callers; i++ {
		go func() {
			<-start
			result, err := cache.GetOrRefresh(input, refresh)
			responses <- response{result: result, err: err}
		}()
	}
	close(start)
	<-refreshStarted
	close(releaseRefresh)

	results := make([]snapshotcache.Result[map[string]models.MetricGroupValue], 0, callers)
	for i := 0; i < callers; i++ {
		response := <-responses
		if response.err != nil {
			t.Fatalf("concurrent getOrRefresh() error = %v", response.err)
		}
		if got := response.result.Value["InnoDB"]["Total Size"]; got != 4*gib {
			t.Fatalf("concurrent InnoDB Total Size = %v, want %d", got, 4*gib)
		}
		if got := response.result.Value["MyISAM"]["Index Size"]; got != uint64(1024) {
			t.Fatalf("concurrent MyISAM Index Size = %v, want 1024", got)
		}
		results = append(results, response.result)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("refresh calls = %d, want 1", got)
	}

	results[0].Value["InnoDB"]["Total Size"] = uint64(7)
	delete(results[0].Value, "MyISAM")
	for i := 1; i < callers; i++ {
		if got := results[i].Value["InnoDB"]["Total Size"]; got != 4*gib {
			t.Fatalf("result %d InnoDB Total Size = %v, want %d", i, got, 4*gib)
		}
		if got := results[i].Value["MyISAM"]["Index Size"]; got != uint64(1024) {
			t.Fatalf("result %d MyISAM Index Size = %v, want 1024", i, got)
		}
	}
}

func TestTableSizeCacheRefreshPanicAllowsLaterRefresh(t *testing.T) {
	const gib = uint64(1024 * 1024 * 1024)
	panicValue := errors.New("first table size refresh panic")
	cache := newTableSizeCache()
	input := Input{
		TotalTables:    tableCount(1001),
		EffectiveRAM:   gib,
		TableThreshold: 1000,
		RAMMultiplier:  4,
		TTL:            time.Hour,
		Now:            time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC),
	}

	var recovered interface{}
	func() {
		defer func() {
			recovered = recover()
		}()
		_, _ = cache.GetOrRefresh(input, func() (map[string]models.MetricGroupValue, error) {
			panic(panicValue)
		})
	}()
	if recovered != panicValue {
		t.Fatalf("leader panic = %v, want original %v", recovered, panicValue)
	}

	type response struct {
		result snapshotcache.Result[map[string]models.MetricGroupValue]
		err    error
	}
	laterDone := make(chan response, 1)
	var laterCalls int32
	go func() {
		result, err := cache.GetOrRefresh(input, func() (map[string]models.MetricGroupValue, error) {
			atomic.AddInt32(&laterCalls, 1)
			return tableSizeTestEngine(5*gib, 1024), nil
		})
		laterDone <- response{result: result, err: err}
	}()

	select {
	case response := <-laterDone:
		if response.err != nil {
			t.Fatalf("later getOrRefresh() error = %v", response.err)
		}
		if got := response.result.Value["InnoDB"]["Total Size"]; got != 5*gib {
			t.Fatalf("later InnoDB Total Size = %v, want %d", got, 5*gib)
		}
	case <-time.After(time.Second):
		t.Fatal("later refresh blocked on poisoned in-flight attempt")
	}
	if got := atomic.LoadInt32(&laterCalls); got != 1 {
		t.Fatalf("later refresh calls = %d, want 1", got)
	}
}

func tableCount(value uint64) *uint64 { return &value }

func newTableSizeCache() *Cache[map[string]models.MetricGroupValue] {
	return New(CloneEngine, func(engine map[string]models.MetricGroupValue) Summary {
		return Summary{TotalSize: EngineTotal(engine)}
	})
}

func TestCacheSupportsStructuredSnapshotsAndLiveZeroCount(t *testing.T) {
	type inventory struct {
		sizes []uint64
	}
	c := New(func(value inventory) inventory {
		value.sizes = slices.Clone(value.sizes)
		return value
	}, func(value inventory) Summary {
		var total uint64
		for _, size := range value.sizes {
			total += size
		}
		return Summary{TotalTables: uint64(len(value.sizes)), TotalSize: total}
	})
	base := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	input := Input{EffectiveRAM: 10, TableThreshold: 1, RAMMultiplier: 4, TTL: time.Hour, Now: base}
	source := inventory{sizes: []uint64{20, 30}}
	refreshFailure := errors.New("refresh failed")
	fail := false
	calls := 0
	refresh := func() (inventory, error) {
		calls++
		if fail {
			return inventory{sizes: []uint64{999}}, refreshFailure
		}
		return source, nil
	}
	first, err := c.GetOrRefresh(input, refresh)
	if err != nil {
		t.Fatal(err)
	}
	first.Value.sizes[0] = 999
	source.sizes[1] = 999
	input.Now = base.Add(time.Hour - time.Second)
	second, err := c.GetOrRefresh(input, refresh)
	if err != nil || calls != 1 || !slices.Equal(second.Value.sizes, []uint64{20, 30}) {
		t.Fatalf("reused snapshot=%+v, calls=%d, error=%v", second, calls, err)
	}
	input.Now = base.Add(time.Hour)
	fail = true
	stale, err := c.GetOrRefresh(input, refresh)
	if err != nil || !stale.UsedStale || !errors.Is(stale.RefreshErr, refreshFailure) || !slices.Equal(stale.Value.sizes, []uint64{20, 30}) {
		t.Fatalf("stale snapshot=%+v, error=%v", stale, err)
	}
	fail = false
	source = inventory{sizes: []uint64{40, 60}}
	if _, err := c.GetOrRefresh(input, refresh); err != nil || calls != 3 {
		t.Fatalf("retry calls=%d, error=%v", calls, err)
	}
	// An explicit live zero count must override the two tables in the snapshot.
	input.TotalTables = tableCount(0)
	input.Now = input.Now.Add(time.Minute)
	if _, err := c.GetOrRefresh(input, refresh); err != nil || calls != 4 {
		t.Fatalf("category change calls=%d, error=%v", calls, err)
	}
	input.Now = input.Now.Add(11*time.Hour + 55*time.Minute - time.Second)
	if _, err := c.GetOrRefresh(input, refresh); err != nil || calls != 4 {
		t.Fatalf("ordinary cache calls=%d, error=%v", calls, err)
	}
	input.Now = input.Now.Add(time.Second)
	if _, err := c.GetOrRefresh(input, refresh); err != nil || calls != 5 {
		t.Fatalf("ordinary cache at 11h55m expiry calls=%d, error=%v", calls, err)
	}
}
