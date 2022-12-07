package metrics

import (
	"github.com/Releem/mysqlconfigurer/releem-agent/config"
	"github.com/advantageous/go-logback/logging"
)

type AgentMetricsGatherer struct {
	logger        logging.Logger
	debug         bool
	configuration *config.Config
}

func NewAgentMetricsGatherer(logger logging.Logger, configuration *config.Config) *AgentMetricsGatherer {

	if logger == nil {
		if configuration.Debug {
			logger = logging.NewSimpleDebugLogger("Agent")
		} else {
			logger = logging.NewSimpleLogger("Agent")
		}
	}

	return &AgentMetricsGatherer{
		logger:        logger,
		debug:         configuration.Debug,
		configuration: configuration,
	}
}

func (Agent *AgentMetricsGatherer) GetMetrics() (Metric, error) {

	output := make(map[string]interface{})

	output["version"] = config.ReleemAgentVersion

	metrics := Metric{"ReleemAgent": output}
	Agent.logger.Debug("collectMetrics ", output)
	return metrics, nil

}
