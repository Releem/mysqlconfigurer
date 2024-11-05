package metrics

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/tasks"
	"github.com/Releem/mysqlconfigurer/utils"
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

func RunWorker(gatherers []models.MetricsGatherer, gatherers_configuration []models.MetricsGatherer, gatherers_query_optimization []models.MetricsGatherer, repeaters models.MetricsRepeater, logger logging.Logger,
	configuration *config.Config, configFile string, Mode models.ModeType) {
	var GenerateTimer, timer, QueryOptimizationTimer *time.Timer
	defer utils.HandlePanic(configuration, logger)
	if logger == nil {
		if configuration.Debug {
			logger = logging.NewSimpleDebugLogger("Worker")
		} else {
			logger = logging.NewSimpleLogger("Worker")
		}
	}

	if (Mode.Name == "Configurations" && Mode.Type != "default") || Mode.Name == "Event" || Mode.Name == "TaskSet" {
		GenerateTimer = time.NewTimer(0 * time.Second)
		timer = time.NewTimer(3600 * time.Second)
	} else {
		GenerateTimer = time.NewTimer(configuration.GenerateConfigPeriod * time.Second)
		timer = time.NewTimer(1 * time.Second)
	}
	QueryOptimizationTimer = time.NewTimer(60 * time.Second)
	QueryOptimizationCollectSqlText := time.NewTimer(1 * time.Second)
	models.SqlText = make(map[string]map[string]string)
	models.SqlTextMutex = sync.RWMutex{}

	if !configuration.QueryOptimization {
		QueryOptimizationTimer.Stop()
		QueryOptimizationCollectSqlText.Stop()
	}
	terminator := makeTerminateChannel()
	for {
		select {
		case <-terminator:
			logger.Info("Exiting")
			os.Exit(0)
		case <-timer.C:
			logger.Println("Starting collection of data for saving a metrics...")
			timer.Reset(configuration.MetricsPeriod * time.Second)
			go func() {
				defer utils.HandlePanic(configuration, logger)
				metrics := utils.CollectMetrics(gatherers, logger, configuration)
				if metrics != nil {
					task := utils.ProcessRepeaters(metrics, repeaters, configuration, logger, models.ModeType{Name: "Metrics", Type: ""})
					if task == "Task" {
						logger.Println(" * A task has been found for the agent...")
						f := tasks.ProcessTaskFunc(metrics, repeaters, gatherers, logger, configuration)
						time.AfterFunc(5*time.Second, f)
					}
				}
				logger.Println("Saved a metrics...")
			}()
			logger.Println("End collection of metrics for saving a metrics...")
		case <-GenerateTimer.C:
			logger.Println("Starting collection of data for generating a config...")
			GenerateTimer.Reset(configuration.GenerateConfigPeriod * time.Second)
			go func() {
				var metrics *models.Metrics
				logger.Println(" * Collecting metrics to recommend a config...")
				defer utils.HandlePanic(configuration, logger)
				if Mode.Name == "TaskSet" && Mode.Type == "queries_optimization" {
					metrics = utils.CollectMetrics(append(gatherers, gatherers_query_optimization...), logger, configuration)
				} else {
					metrics = utils.CollectMetrics(append(gatherers, gatherers_configuration...), logger, configuration)
				}
				if metrics != nil {
					logger.Println(" * Sending metrics to Releem Cloud Platform...")
					utils.ProcessRepeaters(metrics, repeaters, configuration, logger, Mode)
					if Mode.Name == "Configurations" {
						logger.Println("Recommended MySQL configuration downloaded to ", configuration.GetReleemConfDir())
					}
				}
				if (Mode.Name == "Configurations" && Mode.Type != "default") || Mode.Name == "Event" || Mode.Name == "TaskSet" {
					logger.Info("Exiting")
					os.Exit(0)
				}
				logger.Println("Saved a config...")
			}()
			logger.Println("End collection of metrics for saving a metrics...")
		case <-QueryOptimizationTimer.C:
			logger.Println("Starting collection of data for queries optimization...")
			QueryOptimizationTimer.Reset(configuration.QueryOptimizationPeriod * time.Second)
			go func() {
				defer utils.HandlePanic(configuration, logger)
				logger.Println("QueryOptimization")
				metrics := utils.CollectMetrics(append(gatherers, gatherers_query_optimization...), logger, configuration)
				if metrics != nil {
					utils.ProcessRepeaters(metrics, repeaters, configuration, logger, models.ModeType{Name: "Metrics", Type: "QueryOptimization"})
				}
				logger.Println("Saved a queries...")
			}()
		case <-QueryOptimizationCollectSqlText.C:
			QueryOptimizationCollectSqlText.Reset(configuration.QueryOptimizationCollectSqlTextPeriod * time.Second)
			go func() {
				defer utils.HandlePanic(configuration, logger)
				Ready = false
				var SqlText_elem models.SqlTextType
				rows, err := models.DB.Query("SELECT CURRENT_SCHEMA, DIGEST, SQL_TEXT FROM performance_schema.events_statements_history WHERE DIGEST IS NOT NULL AND CURRENT_SCHEMA IS NOT NULL GROUP BY CURRENT_SCHEMA, DIGEST, SQL_TEXT")
				if err != nil {
					logger.Error(err)
				} else {
					for rows.Next() {
						err := rows.Scan(&SqlText_elem.CURRENT_SCHEMA, &SqlText_elem.DIGEST, &SqlText_elem.SQL_TEXT)
						if err != nil {
							logger.Error(err)
						} else {
							models.SqlTextMutex.Lock()
							if models.SqlText[SqlText_elem.CURRENT_SCHEMA] == nil {
								models.SqlText[SqlText_elem.CURRENT_SCHEMA] = make(map[string]string)
							}
							models.SqlText[SqlText_elem.CURRENT_SCHEMA][SqlText_elem.DIGEST] = SqlText_elem.SQL_TEXT
							models.SqlTextMutex.Unlock()
						}
					}
				}

			}()
		}
	}
}
