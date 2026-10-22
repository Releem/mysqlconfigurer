package metrics

import (
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/tasks"
	"github.com/Releem/mysqlconfigurer/utils"
	logging "github.com/google/logger"
)

var Ready bool

// Set up channel on which to send signal notifications.
// We must use a buffered channel or risk missing the signal
// if we're not ready to receive when the signal is sent.
func makeTerminateChannel() chan os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	return ch
}

func RunWorker(gatherers map[string][]models.MetricsGatherer, repeaters models.MetricsRepeater, logger logging.Logger,
	configuration *config.Config, Mode models.ModeType) error {
	defer utils.HandlePanic(configuration, logger)

	var GenerateTimer, timer, QueryOptimizationTimer *time.Timer

	models.SampleQueries = make(map[string]string)
	models.SampleQueriesMutex = sync.RWMutex{}
	terminator := makeTerminateChannel()
	defer signal.Stop(terminator)
	configurationDone := make(chan error, 1)
	oneShot := (Mode.Name == "Configurations" && Mode.Type != "Default") || Mode.Name == "Event" || Mode.Name == "TaskByName"

	if oneShot {
		GenerateTimer = time.NewTimer(1 * time.Second)
		timer = time.NewTimer(24 * time.Hour)
		QueryOptimizationTimer = time.NewTimer(24 * time.Hour)
	} else {
		GenerateTimer = time.NewTimer(configuration.GenerateConfigPeriod * time.Second)
		timer = time.NewTimer(1 * time.Second)
		QueryOptimizationTimer = time.NewTimer(1 * time.Minute)
	}
	CollectSampleQueries := time.NewTimer(1 * time.Second)

	defer GenerateTimer.Stop()
	defer timer.Stop()
	defer QueryOptimizationTimer.Stop()
	defer CollectSampleQueries.Stop()
	if oneShot || !configuration.QueryOptimization {
		CollectSampleQueries.Stop()
	}
	utils.GetStrategyCollectionSampleQueries(configuration, logger, "0")
	for {
		select {
		case <-terminator:
			logger.Info("Exiting")
			return nil
		case err := <-configurationDone:
			if oneShot {
				return err
			}
			if err != nil {
				logger.Error(err)
			}
		case <-timer.C:
			logger.Info("* Starting to collect metrics...")
			timer.Reset(configuration.MetricsPeriod * time.Second)
			go func() {
				defer utils.HandlePanic(configuration, logger)
				metrics := utils.CollectMetrics(append(gatherers["default"], gatherers["metrics"]...), logger, configuration)
				if metrics == nil {
					return
				}
				utils.GetStrategyCollectionSampleQueries(configuration, logger, utils.ConvertUptimeToStr(metrics.DB.Metrics.Status))
				response, err := utils.ProcessRepeaters(metrics, repeaters, configuration, logger, models.ModeType{Name: "Metrics", Type: ""})
				if err != nil {
					logger.Error("Sending metrics failed: ", err)
					return
				}
				if string(response) == "Task" {
					logger.Info("* A task received by the agent...")
					f := tasks.ProcessTaskFunc(repeaters, gatherers["default"], logger, configuration)
					time.AfterFunc(5*time.Second, f)
				}

				logger.Info("* Database Metrics are saved...")
			}()
		case <-GenerateTimer.C:
			logger.Info("* Starting to collect metrics...")
			if !oneShot {
				GenerateTimer.Reset(configuration.GenerateConfigPeriod * time.Second)
			}
			go func() {
				err := errors.New("configuration collection did not complete")
				defer func() { configurationDone <- err }()
				defer utils.HandlePanic(configuration, logger)
				err = collectConfiguration(gatherers, repeaters, logger, configuration, Mode)
			}()
		case <-QueryOptimizationTimer.C:
			logger.Info("* Starting to collect Database metrics for Query Analytics...")
			QueryOptimizationTimer.Reset(configuration.QueryOptimizationPeriod * time.Second)
			go func() {
				defer utils.HandlePanic(configuration, logger)
				metrics := utils.CollectMetrics(append(gatherers["default"], gatherers["query_optimization"]...), logger, configuration)
				if metrics == nil {
					return
				}
				if _, err := utils.ProcessRepeaters(metrics, repeaters, configuration, logger, models.ModeType{Name: "Metrics", Type: "Queries"}); err != nil {
					logger.Error("Sending query metrics failed: ", err)
					return
				}
				logger.Info("* Database metrics for Query Analytics are saved...")
			}()
		case <-CollectSampleQueries.C:
			CollectSampleQueries.Reset(configuration.CollectSampleQueriesPeriod * time.Second)
			go func() {
				defer utils.HandlePanic(configuration, logger)
				utils.CollectMetrics(gatherers["sample_queries"], logger, configuration)
			}()
		}
	}
}

func collectConfiguration(gatherers map[string][]models.MetricsGatherer, repeaters models.MetricsRepeater, logger logging.Logger, configuration *config.Config, mode models.ModeType) error {
	group := "configuration"
	if mode.Name == "TaskByName" && mode.Type == "queries_optimization" {
		group = "query_optimization"
	}
	logger.Info("* Collecting metrics...")
	collected := utils.CollectMetrics(append(gatherers["default"], gatherers[group]...), logger, configuration)
	if collected == nil {
		return errors.New("configuration metrics unavailable")
	}
	utils.GetStrategyCollectionSampleQueries(configuration, logger, "0")
	logger.Info("* Sending metrics to the Releem Cloud Platform...")
	_, err := utils.ProcessRepeaters(collected, repeaters, configuration, logger, mode)
	if err != nil {
		return err
	}

	if mode.Name == "Configurations" {
		logger.Info("* The recommended Database configuration has been downloaded to: ", configuration.GetReleemConfDir())
	}
	logger.Info("* Database Metrics are saved...")
	return nil
}
