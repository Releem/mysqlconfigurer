// Package tablesize applies the same table-size cache policy to every DBMS.
package tablesize

import (
	"math"
	"time"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils/cache"
)

// Summary describes the table count and size in bytes in a complete snapshot.
type Summary struct {
	TotalTables uint64
	TotalSize   uint64
}

// Input supplies current server information and the large-server cache settings.
type Input struct {
	// TotalTables overrides the snapshot count when a DBMS collects it live.
	// A nil pointer uses the count from the cached snapshot instead.
	TotalTables    *uint64
	EffectiveRAM   uint64
	TableThreshold int64
	RAMMultiplier  int64
	TTL            time.Duration
	Now            time.Time
}

// InputFromMetrics normalizes server RAM and the configured TTL for both DBMSs.
// The default table count comes from the snapshot; callers with a live count
// can set TotalTables on the returned input.
func InputFromMetrics(metrics *models.Metrics, cfg *config.Config, now time.Time) Input {
	return Input{
		EffectiveRAM:   EffectiveRAM(metrics),
		TableThreshold: cfg.TableSizeCacheTableThreshold,
		RAMMultiplier:  cfg.TableSizeCacheRAMMultiplier,
		TTL:            TTLFromSeconds(cfg.TableSizeCacheTTL),
		Now:            now,
	}
}

type snapshot[T any] struct {
	value       T
	summary     Summary
	largeServer bool
}

// Cache stores complete table-size data of any type using a shared size policy.
// Create it with New and do not copy it after use.
type Cache[T any] struct {
	snapshots *cache.Cache[snapshot[T]]
	summarize func(T) Summary
}

// New creates a cache. clone must copy all mutable references in the data;
// summarize must report its count and total size without modifying it.
func New[T any](clone func(T) T, summarize func(T) Summary) *Cache[T] {
	if clone == nil || summarize == nil {
		panic("tablesize: clone and summarize functions are required")
	}
	return &Cache[T]{
		snapshots: cache.New(func(value snapshot[T]) snapshot[T] {
			value.value = clone(value.value)
			return value
		}),
		summarize: summarize,
	}
}

// Peek reads the last successful table-size snapshot without refreshing it.
// Expired snapshots remain available to minute reports until a full refresh.
func (c *Cache[T]) Peek() (T, bool) {
	value, ok := c.snapshots.Peek()
	return value.value, ok
}

// GetOrRefresh uses 11h55m for ordinary servers and Input.TTL for large ones.
// A changed size category forces a refresh. Failures retain the last complete
// snapshot and retry on the next call; a first failure returns an error.
func (c *Cache[T]) GetOrRefresh(input Input, refresh func() (T, error)) (cache.Result[T], error) {
	if input.TotalTables != nil {
		count := *input.TotalTables
		input.TotalTables = &count
	}
	result, err := c.snapshots.GetOrRefresh(input.Now, cache.Policy[snapshot[T]]{
		TTL: func(value snapshot[T]) time.Duration {
			if value.largeServer {
				return input.TTL
			}
			return 12*time.Hour - 5*time.Minute
		},
		Reusable: func(value snapshot[T]) bool {
			return eligible(input, value.summary) == value.largeServer
		},
	}, func() (snapshot[T], error) {
		value, err := refresh()
		if err != nil {
			return snapshot[T]{}, err
		}
		summary := c.summarize(value)
		return snapshot[T]{value: value, summary: summary, largeServer: eligible(input, summary)}, nil
	})
	if err != nil {
		return cache.Result[T]{}, err
	}
	return cache.Result[T]{Value: result.Value.value, UsedStale: result.UsedStale, RefreshErr: result.RefreshErr}, nil
}

func eligible(input Input, summary Summary) bool {
	if input.EffectiveRAM == 0 || input.TableThreshold < 0 || input.RAMMultiplier < 0 {
		return false
	}
	count := summary.TotalTables
	if input.TotalTables != nil {
		count = *input.TotalTables
	}
	if count <= uint64(input.TableThreshold) {
		return false
	}
	multiplier := uint64(input.RAMMultiplier)
	if multiplier > math.MaxUint64/input.EffectiveRAM {
		return false
	}
	return summary.TotalSize > input.EffectiveRAM*multiplier
}
