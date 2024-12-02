package models

import (
	"database/sql"
	"sync"

	"github.com/Releem/mysqlconfigurer/config"
)

type MetricType byte
type MetricIntervalType byte

type MetricValue struct {
	Name  string
	Value string
}
type MetricGroupValue map[string]interface{}

type ModeType struct {
	Name string
	Type string
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
			TotalTables        int
			TotalMyisamIndexes int
			Engine             map[string]MetricGroupValue
			Latency            string
			Databases          []string
			InnoDBEngineStatus string
		}
		Conf struct {
			Variables MetricGroupValue
		}
		Info                MetricGroupValue
		Queries             []MetricGroupValue
		QueriesOptimization map[string][]MetricGroupValue
	}
	ReleemAgent struct {
		Info  MetricGroupValue
		Tasks MetricGroupValue
		Conf  config.Config
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
	ProcessMetrics(context MetricContext, metrics Metrics, Mode ModeType) (interface{}, error)
}

type SqlTextType struct {
	CURRENT_SCHEMA string
	DIGEST         string
	SQL_TEXT       string
}

var (
	DB           *sql.DB
	SqlText      map[string]map[string]string
	SqlTextMutex sync.RWMutex
)
