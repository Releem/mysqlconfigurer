package cache

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

type tableSnapshot struct {
	Sizes map[string][]uint64
	Count uint64
}

func cloneTables(value tableSnapshot) tableSnapshot {
	result := tableSnapshot{Count: value.Count, Sizes: make(map[string][]uint64, len(value.Sizes))}
	for name, sizes := range value.Sizes {
		result.Sizes[name] = append([]uint64(nil), sizes...)
	}
	return result
}

func tablePolicy() Policy[tableSnapshot] {
	return Policy[tableSnapshot]{TTL: func(tableSnapshot) time.Duration { return 12 * time.Hour }}
}

func TestCacheRefreshesAtExpiryAndIsolatesSnapshots(t *testing.T) {
	c := New(cloneTables)
	now := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	source := tableSnapshot{Sizes: map[string][]uint64{"app": {100}}, Count: 1}
	calls := 0
	refresh := func() (tableSnapshot, error) { calls++; return source, nil }
	first, err := c.GetOrRefresh(now, tablePolicy(), refresh)
	if err != nil {
		t.Fatal(err)
	}
	first.Value.Sizes["app"][0] = 999
	source.Sizes["app"][0] = 200
	second, err := c.GetOrRefresh(now.Add(12*time.Hour-time.Second), tablePolicy(), refresh)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || second.Value.Sizes["app"][0] != 100 {
		t.Fatalf("cached value = %+v, refresh calls = %d", second, calls)
	}
	third, err := c.GetOrRefresh(now.Add(12*time.Hour), tablePolicy(), refresh)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || third.Value.Sizes["app"][0] != 200 {
		t.Fatalf("refreshed value = %+v, refresh calls = %d", third, calls)
	}
}

func TestCachePropagatesInitialErrorAndRetriesStaleRefresh(t *testing.T) {
	c := New(cloneTables)
	now := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	failure := errors.New("database unavailable")
	fail := func() (tableSnapshot, error) { return tableSnapshot{Count: 99}, failure }
	if _, err := c.GetOrRefresh(now, tablePolicy(), fail); !errors.Is(err, failure) {
		t.Fatalf("initial error = %v", err)
	}
	refresh := func() (tableSnapshot, error) { return tableSnapshot{Count: 1}, nil }
	if _, err := c.GetOrRefresh(now, tablePolicy(), refresh); err != nil {
		t.Fatal(err)
	}
	now = now.Add(12 * time.Hour)
	stale, err := c.GetOrRefresh(now, tablePolicy(), fail)
	if err != nil || !stale.UsedStale || !errors.Is(stale.RefreshErr, failure) || stale.Value.Count != 1 {
		t.Fatalf("stale result = %+v, error = %v", stale, err)
	}
	retried := false
	fresh, err := c.GetOrRefresh(now, tablePolicy(), func() (tableSnapshot, error) { retried = true; return tableSnapshot{Count: 2}, nil })
	if err != nil || !retried || fresh.UsedStale || fresh.Value.Count != 2 {
		t.Fatalf("retry = %+v, error = %v, refreshed = %v", fresh, err, retried)
	}
}

func TestCacheSupportsSlicesAndCallerReusePolicy(t *testing.T) {
	c := New(func(value []string) []string { return append([]string(nil), value...) })
	now := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	policy := Policy[[]string]{TTL: func([]string) time.Duration { return time.Hour }}
	if _, err := c.GetOrRefresh(now, policy, func() ([]string, error) { return []string{"old"}, nil }); err != nil {
		t.Fatal(err)
	}
	policy.Reusable = func(value []string) bool { return reflect.DeepEqual(value, []string{"new"}) }
	result, err := c.GetOrRefresh(now, policy, func() ([]string, error) { return []string{"new"}, nil })
	if err != nil || !reflect.DeepEqual(result.Value, []string{"new"}) {
		t.Fatalf("replacement = %+v, error = %v", result, err)
	}
}

func TestCacheConcurrentCallersShareRefresh(t *testing.T) {
	c := New(cloneTables)
	now := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	const callers = 20
	var wg sync.WaitGroup
	calls := 0 // Refresh callbacks are serialized by the cache.
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := c.GetOrRefresh(now, tablePolicy(), func() (tableSnapshot, error) {
				calls++
				return tableSnapshot{Sizes: map[string][]uint64{"app": {100}}}, nil
			})
			if err != nil {
				t.Error(err)
				return
			}
			if result.Value.Sizes["app"][0] != 100 {
				t.Error("shared mutable snapshot")
			}
			result.Value.Sizes["app"][0] = 999
		}()
	}
	wg.Wait()
	if calls != 1 {
		t.Fatalf("refresh calls = %d, want 1", calls)
	}
}

func TestCacheNilSnapshotIsAValidSuccess(t *testing.T) {
	c := New(func(value []string) []string { return append([]string(nil), value...) })
	now := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	policy := Policy[[]string]{TTL: func([]string) time.Duration { return time.Hour }}
	calls := 0
	refresh := func() ([]string, error) { calls++; return nil, nil }
	for i := 0; i < 2; i++ {
		result, err := c.GetOrRefresh(now, policy, refresh)
		if err != nil || result.Value != nil || result.UsedStale {
			t.Fatalf("nil snapshot result = %+v, error = %v", result, err)
		}
	}
	if calls != 1 {
		t.Fatalf("refresh calls = %d, want 1", calls)
	}
}

func TestCacheRefreshPanicDoesNotLeaveLockHeld(t *testing.T) {
	c := New(cloneTables)
	now := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected refresh panic")
			}
		}()
		_, _ = c.GetOrRefresh(now, tablePolicy(), func() (tableSnapshot, error) { panic("failed refresh") })
	}()
	done := make(chan error, 1)
	go func() {
		_, err := c.GetOrRefresh(now, tablePolicy(), func() (tableSnapshot, error) { return tableSnapshot{Count: 1}, nil })
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cache lock held after panic")
	}
}

func TestPeekDoesNotRefreshOrWaitForRefresh(t *testing.T) {
	c := New(cloneTables)
	if _, ok := c.Peek(); ok {
		t.Fatal("empty cache returned a snapshot")
	}
	now := time.Now()
	if _, err := c.GetOrRefresh(now, tablePolicy(), func() (tableSnapshot, error) { return tableSnapshot{Count: 1}, nil }); err != nil {
		t.Fatal(err)
	}
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		_, _ = c.GetOrRefresh(now.Add(24*time.Hour), tablePolicy(), func() (tableSnapshot, error) { close(started); <-release; return tableSnapshot{Count: 2}, nil })
	}()
	<-started
	read := make(chan tableSnapshot, 1)
	go func() { value, _ := c.Peek(); read <- value }()
	select {
	case value := <-read:
		if value.Count != 1 {
			t.Errorf("Peek during refresh count=%d, want 1", value.Count)
		}
	case <-time.After(time.Second):
		t.Error("Peek waited for database refresh")
	}
	close(release)
	<-done
	value, ok := c.Peek()
	if !ok || value.Count != 2 {
		t.Fatalf("Peek after refresh=%+v/%v", value, ok)
	}
}
