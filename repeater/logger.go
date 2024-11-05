package repeater

import (
	"github.com/Releem/mysqlconfigurer/models"
	lg "github.com/advantageous/go-logback/logging"
)

type LogMetricsRepeater struct {
	logger lg.Logger
}

func (lr LogMetricsRepeater) ProcessMetrics(metrics models.Metric) error {
	for _, m := range metrics {
		lr.logger.Printf("%s", m)
	}
	return nil
}

func NewLogMetricsRepeater() LogMetricsRepeater {
	logger := lg.NewSimpleLogger("log-repeater")
	return LogMetricsRepeater{logger}
}
