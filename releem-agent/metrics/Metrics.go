package metrics

type MetricType byte
type MetricIntervalType byte

type MetricValue struct {
	name  string
	value string
}
type MetricGroupValue map[string]interface{}

type Metrics struct {
	Hostname  string
	Timestamp uint64
	System    struct {
		Info MetricGroupValue
		Conf struct {
		}
		Metrics struct {
			DiskIO         []MetricGroupValue
			FileSystem     []MetricGroupValue
			PhysicalMemory MetricGroupValue
			CPU            MetricGroupValue
			IOPS           MetricGroupValue
		}
	}
	DB struct {
		Metrics struct {
			Status             MetricGroupValue
			TotalTables        string
			TotalMyisamIndexes string
			Engine             map[string]MetricGroupValue
			Latency            string
		}
		Conf struct {
			Variables MetricGroupValue
		}
		Info MetricGroupValue
	}
	ReleemAgent struct {
		Info MetricGroupValue
	}
}

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
	GetMemoryLimit() int
	GetReleemConfDir() string
}

type MetricsGatherer interface {
	GetMetrics(metrics *Metrics) error
}

type MetricsRepeater interface {
	ProcessMetrics(context MetricContext, metrics Metrics) error
}
