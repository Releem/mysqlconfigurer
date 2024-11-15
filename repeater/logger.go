package repeater

import (
	"io"
	"log"

	"github.com/Releem/mysqlconfigurer/models"
	logging "github.com/google/logger"
)

type LogMetricsRepeater struct {
	logger logging.Logger
}

func (lr LogMetricsRepeater) ProcessMetrics(metrics models.Metric) error {
	for _, m := range metrics {
		lr.logger.Infof("%s", m)
	}
	return nil
}

func NewLogMetricsRepeater() LogMetricsRepeater {
	logger := *logging.Init("releem-agent", true, false, io.Discard)
	defer logger.Close()
	logging.SetFlags(log.LstdFlags | log.Lshortfile)
	return LogMetricsRepeater{logger}
}
