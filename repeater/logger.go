package repeater

import (
	m "github.com/Releem/mysqlconfigurer/metrics"
	lg "github.com/advantageous/go-logback/logging"
)

type LogMetricsRepeater struct {
	logger lg.Logger
}

func (lr LogMetricsRepeater) ProcessMetrics(metrics m.Metric) error {
	for _, m := range metrics {
		lr.logger.Printf("%s", m)
	}
	return nil
}

func NewLogMetricsRepeater() LogMetricsRepeater {
	logger := lg.NewSimpleLogger("log-repeater")
	return LogMetricsRepeater{logger}
}
