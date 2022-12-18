package metrics

type MetricType byte
type MetricIntervalType byte

type MetricValue struct {
	name  string
	value string
}
type MetricGroupValue map[string]interface{}

type Metrics struct {
	System struct {
		Info map[string]interface{}
		Conf struct {
		}
		Metrics struct {
			DiskIO         []map[string]interface{}
			FileSystem     []map[string]interface{}
			PhysicalMemory map[string]interface{}
			CPU            map[string]interface{}
			IOPS           map[string]interface{}
		}
	}
	DB struct {
		Metrics struct {
			Status             map[string]string
			TotalTables        string
			TotalMyisamIndexes string
			Engine             map[string]map[string]string
			Latency            string
		}
		Conf struct {
			Variables map[string]string
		}
		Info map[string]interface{}
	}
	ReleemAgent struct {
		Info map[string]interface{}
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
