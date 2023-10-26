package metrics

type MetricType byte
type MetricIntervalType byte

type MetricValue struct {
	name  string
	value string
}
type MetricGroupValue map[string]interface{}

type Mode struct {
	Name     string
	ModeType string
}
type Metrics struct {
	System struct {
		Info    MetricGroupValue
		Conf    MetricGroupValue
		Metrics MetricGroupValue
	}
	DB struct {
		Metrics struct {
			Status             MetricGroupValue
			TotalTables        string
			TotalMyisamIndexes string
			Engine             map[string]MetricGroupValue
			Latency            string
			Databases          []string
		}
		Conf struct {
			Variables MetricGroupValue
		}
		Info MetricGroupValue
	}
	ReleemAgent struct {
		Info  MetricGroupValue
		Tasks MetricGroupValue
	}
}

type Metric map[string]MetricGroupValue

// type Metric interface {
// 	// GetProvider() string
// 	// GetType() MetricType
// 	// GetValue() string
// 	// GetName() string
// }

type Task struct {
	TaskID     *int    `json:"task_id"`
	TaskTypeID *int    `json:"task_type_id"`
	IsExist    *string `json:"is_exist"`
}

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
	ProcessMetrics(context MetricContext, metrics Metrics) (interface{}, error)
}

func MapJoin(map1, map2 MetricGroupValue) MetricGroupValue {
	for k, v := range map2 {
		map1[k] = v
	}
	return map1
}
