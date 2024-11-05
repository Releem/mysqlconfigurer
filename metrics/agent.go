package metrics

import (
	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
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

func (Agent *AgentMetricsGatherer) GetMetrics(metrics *models.Metrics) error {
	defer utils.HandlePanic(Agent.configuration, Agent.logger)

	output := make(map[string]interface{})
	output["Version"] = config.ReleemAgentVersion
	if len(Agent.configuration.Hostname) > 0 {
		output["Hostname"] = Agent.configuration.Hostname
	}
	output["QueryOptimization"] = Agent.configuration.QueryOptimization
	models.SqlTextMutex.RLock()
	output["QueryOptimizationSqlTextCount"] = len(models.SqlText)
	models.SqlTextMutex.RUnlock()

	metrics.ReleemAgent.Info = output

	Agent.logger.Debug("CollectMetrics  ", output)
	return nil

}
