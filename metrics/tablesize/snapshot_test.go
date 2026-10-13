package tablesize

import (
	"errors"
	"slices"
	"testing"
	"time"
)

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
