package metrics

type MetricType byte
type MetricIntervalType byte

type MetricValue struct {
	name  string
	value string
}
type MetricGroupValue map[string]string

type Metric map[string]MetricGroupValue

// type Metric interface {
// 	// GetProvider() string
// 	// GetType() MetricType
// 	// GetValue() string
// 	// GetName() string
// }

type MetricContext interface {
	GetApiKey() string
	GetEnv() string
}

type MetricsGatherer interface {
	GetMetrics() (Metric, error)
}

type MetricsRepeater interface {
	ProcessMetrics(context MetricContext, metrics Metric) error
}
