package tablesize

import (
	"github.com/Releem/mysqlconfigurer/models"
	"math"
	"testing"
	"time"
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

func TestEngineTableCount(t *testing.T) {
	engine := map[string]models.MetricGroupValue{
		"BASE TABLE": {"Table Number": uint64(7)},
		"VIEW":       {"Table Number": uint64(3)},
	}
	if got := EngineTableCount(engine); got != 10 {
		t.Fatalf("count = %d, want 10", got)
	}
	if got := EngineTableCount(nil); got != 0 {
		t.Fatalf("empty count = %d", got)
	}
}
