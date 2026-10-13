package tablesize

import (
	"errors"
	"math"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Releem/mysqlconfigurer/models"
	snapshotcache "github.com/Releem/mysqlconfigurer/utils/cache"
)

func TestEffectiveTableSizeRAM(t *testing.T) {
	const gib = uint64(1024 * 1024 * 1024)
	const kib = uint64(1024)

	tests := []struct {
		name          string
		physicalTotal interface{}
		instanceType  string
		want          uint64
	}{
		{
			name:          "physical with zero memory limit",
			physicalTotal: uint64(8 * gib),
			want:          8 * gib,
		},
		{
			name:          "physical is not capped by memory limit",
			physicalTotal: uint64(8 * gib),
			want:          8 * gib,
		},
		{
			name:          "physical below memory limit",
			physicalTotal: uint64(gib),
			want:          gib,
		},
		{
			name: "memory limit does not replace missing physical",
			want: 0,
		},
		{
			name: "physical missing with zero memory limit",
			want: 0,
		},
		{
			name:          "uint64 physical value",
			physicalTotal: uint64(8 * gib),
			want:          8 * gib,
		},
		{
			name:          "int physical value",
			physicalTotal: int(8 * gib),
			want:          8 * gib,
		},
		{
			name:          "float64 physical value",
			physicalTotal: float64(8 * gib),
			want:          8 * gib,
		},
		{
			name:          "numeric string physical value",
			physicalTotal: "8589934592",
			want:          8 * gib,
		},
		{
			name:          "aws/rds kilobytes are converted to bytes",
			physicalTotal: int(8 * gib / kib),
			instanceType:  "aws/rds",
			want:          8 * gib,
		},
		{
			name:          "non-aws instance type keeps bytes",
			physicalTotal: uint64(8 * gib),
			instanceType:  "gcp/cloudsql",
			want:          8 * gib,
		},
		{
			name:          "aws/rds overflow saturates",
			physicalTotal: uint64(math.MaxUint64),
			instanceType:  "aws/rds",
			want:          math.MaxUint64,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := &models.Metrics{}
			if tt.physicalTotal != nil || tt.instanceType != "" {
				metrics.System.Info = models.MetricGroupValue{}
			}
			if tt.physicalTotal != nil {
				metrics.System.Info["PhysicalMemory"] = models.MetricGroupValue{
					"total": tt.physicalTotal,
				}
			}
			if tt.instanceType != "" {
				metrics.System.Info["Host"] = models.MetricGroupValue{
					"InstanceType": tt.instanceType,
				}
			}
			got := EffectiveRAM(metrics)

			if got != tt.want {
				t.Fatalf("EffectiveRAM() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestTableSizeUint64AcceptsMetricNumberRepresentations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value interface{}
		want  uint64
	}{
		{name: "uint", value: uint(1), want: 1},
		{name: "uint8", value: uint8(2), want: 2},
		{name: "uint16", value: uint16(3), want: 3},
		{name: "uint32", value: uint32(4), want: 4},
		{name: "uint64", value: uint64(math.MaxUint64), want: math.MaxUint64},
		{name: "int", value: int(5), want: 5},
		{name: "int8", value: int8(6), want: 6},
		{name: "int16", value: int16(7), want: 7},
		{name: "int32", value: int32(8), want: 8},
		{name: "int64", value: int64(9), want: 9},
		{name: "float32 truncates fractional bytes", value: float32(10.75), want: 10},
		{name: "float64 truncates fractional bytes", value: float64(11.75), want: 11},
		{name: "integer string", value: " 12 ", want: 12},
		{name: "decimal string", value: "13.75", want: 13},
		{name: "positive infinity saturates", value: math.Inf(1), want: math.MaxUint64},
		{name: "zero signed value", value: int64(0), want: 0},
		{name: "negative signed value", value: int64(-1), want: 0},
		{name: "negative float", value: float64(-1), want: 0},
		{name: "NaN", value: math.NaN(), want: 0},
		{name: "negative infinity", value: math.Inf(-1), want: 0},
		{name: "invalid string", value: "twelve", want: 0},
		{name: "unsupported type", value: []byte("14"), want: 0},
		{name: "nil", value: nil, want: 0},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Uint64(tt.value); got != tt.want {
				t.Fatalf("Uint64(%#v) = %d, want %d", tt.value, got, tt.want)
			}
		})
	}
}

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

func TestTableSizeCacheTTLFromSecondsSaturates(t *testing.T) {
	tests := []struct {
		name    string
		seconds time.Duration
		want    time.Duration
	}{
		{
			name:    "normal duration",
			seconds: 604800,
			want:    7 * 24 * time.Hour,
		},
		{
			name:    "extreme duration",
			seconds: time.Duration(math.MaxInt64),
			want:    time.Duration(math.MaxInt64),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TTLFromSeconds(tt.seconds); got != tt.want {
				t.Fatalf("TTLFromSeconds(%d) = %d, want %d", tt.seconds, got, tt.want)
			}
		})
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

func TestTableSizeCacheTTLFreshOneSecondBeforeExpiry(t *testing.T) {
	const gib = uint64(1024 * 1024 * 1024)
	collectedAt := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
	cache := newTableSizeCache()
	calls := 0
	refresh := func() (map[string]models.MetricGroupValue, error) {
		calls++
		return tableSizeTestEngine(4*gib, 1), nil
	}
	input := Input{
		TotalTables:    tableCount(1001),
		EffectiveRAM:   gib,
		TableThreshold: 1000,
		RAMMultiplier:  4,
		TTL:            24 * time.Hour,
		Now:            collectedAt,
	}

	if _, err := cache.GetOrRefresh(input, refresh); err != nil {
		t.Fatalf("first getOrRefresh() error = %v", err)
	}
	input.Now = collectedAt.Add(24*time.Hour - time.Second)
	if _, err := cache.GetOrRefresh(input, refresh); err != nil {
		t.Fatalf("second getOrRefresh() error = %v", err)
	}

	if calls != 1 {
		t.Fatalf("refresh calls = %d, want 1", calls)
	}
}

func TestTableSizeCacheTTLExactlyAtExpiryRefreshes(t *testing.T) {
	const gib = uint64(1024 * 1024 * 1024)
	collectedAt := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
	cache := newTableSizeCache()
	calls := 0
	refresh := func() (map[string]models.MetricGroupValue, error) {
		calls++
		return tableSizeTestEngine(4*gib, 1), nil
	}
	input := Input{
		TotalTables:    tableCount(1001),
		EffectiveRAM:   gib,
		TableThreshold: 1000,
		RAMMultiplier:  4,
		TTL:            24 * time.Hour,
		Now:            collectedAt,
	}

	if _, err := cache.GetOrRefresh(input, refresh); err != nil {
		t.Fatalf("first getOrRefresh() error = %v", err)
	}
	input.Now = collectedAt.Add(24 * time.Hour)
	if _, err := cache.GetOrRefresh(input, refresh); err != nil {
		t.Fatalf("second getOrRefresh() error = %v", err)
	}

	if calls != 2 {
		t.Fatalf("refresh calls = %d, want 2", calls)
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
