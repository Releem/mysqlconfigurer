package metrics

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/Releem/mysqlconfigurer/releem-agent/config"
	"github.com/advantageous/go-logback/logging"
)

var Ready bool

// Set up channel on which to send signal notifications.
// We must use a buffered channel or risk missing the signal
// if we're not ready to receive when the signal is sent.
func makeTerminateChannel() <-chan os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	return ch
}

func RunWorker(gatherers []MetricsGatherer, repeaters []MetricsRepeater, logger logging.Logger,
	configuration *config.Config, configFile string, ReleemAgentVersion string) {

	if logger == nil {
		if configuration.Debug {
			logger = logging.NewSimpleDebugLogger("Worker")
		} else {
			logger = logging.NewSimpleLogger("Worker")
		}
	}
	timer := time.NewTimer(configuration.TimePeriodSeconds * time.Second)
	configTimer := time.NewTimer(configuration.ReadConfigSeconds * time.Second)
	GenerateTimer := time.NewTimer(configuration.GenerateConfigSeconds * time.Second)

	// var configuration *config.Config
	// if newConfig, err := config.LoadConfig(configFile, logger); err != nil {
	// 	logger.PrintError("Error reading config", err)
	// } else {
	// 	configuration = newConfig
	// }
	terminator := makeTerminateChannel()

	for {
		select {
		case <-terminator:
			logger.Info("Exiting")
			os.Exit(0)
		case <-timer.C:
			Ready = false
			logger.Debug("Timer collect metrics tick")
			timer.Reset(configuration.TimePeriodSeconds * time.Second)
			metrics := collectMetrics(gatherers, logger)
			if Ready {
				processMetrics(metrics, repeaters, configuration, logger)
			}
		case <-configTimer.C:
			configTimer.Reset(configuration.ReadConfigSeconds * time.Second)
			if newConfig, err := config.LoadConfig(configFile, logger); err != nil {
				logger.PrintError("Error reading config", err)
			} else {
				configuration = newConfig
				logger.Debug("LOADED NEW CONFIG", "APIKEY", configuration.GetApiKey())
			}
		case <-GenerateTimer.C:
			logger.Println("Generating the recommended configuration")
			cmd := exec.Command("/bin/bash", "/opt/releem/mysqlconfigurer.sh")
			cmd.Env = os.Environ()
			cmd.Env = append(cmd.Env, "PATH=/bin:/sbin:/usr/bin:/usr/sbin")
			stdout, err := cmd.Output()
			if err != nil {
				logger.PrintError("Config generation with error", err)
			}
			logger.Debug(string(stdout))
			GenerateTimer.Reset(configuration.GenerateConfigSeconds * time.Second)
		}
	}
}

func processMetrics(metrics Metric, repeaters []MetricsRepeater,
	configuration *config.Config, logger logging.Logger) {
	for _, r := range repeaters {
		if err := r.ProcessMetrics(configuration, metrics); err != nil {
			logger.PrintError("Repeater failed", err)
		}
	}
}

func collectMetrics(gatherers []MetricsGatherer, logger logging.Logger) Metric {
	metrics := make(Metric)
	for _, g := range gatherers {
		m, err := g.GetMetrics()
		if err != nil {
			logger.Error("Problem getting metrics from gatherer")
			return Metric{}
		}
		for k, v := range m {
			if len(v) == 0 {
				_, found := metrics[k]
				if !found {
					metrics[k] = make(MetricGroupValue)
				}
			} else {
				for k1, v1 := range v {
					_, found := metrics[k]
					if !found {
						metrics[k] = make(MetricGroupValue)
					}
					metrics[k][k1] = v1
				}
			}
		}
	}
	Ready = true
	return metrics
}
