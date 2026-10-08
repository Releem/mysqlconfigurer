package tablesize

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
)

// EffectiveRAM returns physical RAM in bytes, normalizing AWS RDS KiB metrics.
func EffectiveRAM(metrics *models.Metrics) uint64 {
	if metrics == nil {
		return 0
	}
	memory, ok := metrics.System.Info["PhysicalMemory"].(models.MetricGroupValue)
	if !ok {
		return 0
	}
	total := Uint64(memory["total"])
	if total == 0 {
		return 0
	}
	// AWS RDS enhanced OS metrics report memory.total in kilobytes; local/GCP use bytes.
	if host, ok := metrics.System.Info["Host"].(models.MetricGroupValue); ok {
		if instanceType, _ := host["InstanceType"].(string); instanceType == "aws/rds" {
			const kib = uint64(1024)
			if total > math.MaxUint64/kib {
				return math.MaxUint64
			}
			return total * kib
		}
	}
	return total
}

// Uint64 converts a numeric metric to a nonnegative size, saturating overflow.
func Uint64(value interface{}) uint64 {
	if text, isString := value.(string); isString {
		text = strings.TrimSpace(text)
		if parsed, err := strconv.ParseUint(text, 10, 64); err == nil {
			return parsed
		}
		if parsed, err := strconv.ParseFloat(text, 64); err == nil {
			return sizeFloat64(parsed)
		}
		return 0
	}

	switch number := utils.AsNumber(value); number.Kind {
	case utils.UnsignedNumber:
		return number.Uint
	case utils.SignedNumber:
		// A negative size is nonsense from information_schema; treat it as 0.
		if number.Int > 0 {
			return uint64(number.Int)
		}
	case utils.FloatNumber:
		return sizeFloat64(number.Float)
	}
	return 0
}

func sizeFloat64(value float64) uint64 {
	if value <= 0 || math.IsNaN(value) {
		return 0
	}
	if value >= float64(math.MaxUint64) {
		return math.MaxUint64
	}
	return uint64(value)
}

// TTLFromSeconds converts the configured seconds to a duration without overflow.
func TTLFromSeconds(seconds time.Duration) time.Duration {
	if seconds <= 0 {
		return 0
	}
	const maxSeconds = time.Duration(math.MaxInt64) / time.Second
	if seconds > maxSeconds {
		return time.Duration(math.MaxInt64)
	}
	return seconds * time.Second
}

// EngineTotal sums table sizes across engines or table types, saturating overflow.
func EngineTotal(engine map[string]models.MetricGroupValue) uint64 {
	var total uint64
	for _, metrics := range engine {
		size := Uint64(metrics["Total Size"])
		if size > math.MaxUint64-total {
			return math.MaxUint64
		}
		total += size
	}
	return total
}

// CloneEngine copies engine or table-type maps containing scalar metric values.
func CloneEngine(engine map[string]models.MetricGroupValue) map[string]models.MetricGroupValue {
	if engine == nil {
		return nil
	}

	cloned := make(map[string]models.MetricGroupValue, len(engine))
	for name, metrics := range engine {
		clonedMetrics := make(models.MetricGroupValue, len(metrics))
		for key, value := range metrics {
			clonedMetrics[key] = value
		}
		cloned[name] = clonedMetrics
	}
	return cloned
}

// EngineTableCount derives the total count from per-engine/type aggregates.
func EngineTableCount(engine map[string]models.MetricGroupValue) uint64 {
	var total uint64
	for _, metrics := range engine {
		total += Uint64(metrics["Table Number"])
	}
	return total
}
