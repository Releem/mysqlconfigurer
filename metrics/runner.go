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
	logging "github.com/google/logger"
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

func RunWorker(gatherers []models.MetricsGatherer, gatherers_metrics []models.MetricsGatherer, gatherers_configuration []models.MetricsGatherer, gatherers_query_optimization []models.MetricsGatherer, repeaters models.MetricsRepeater, logger logging.Logger,
	configuration *config.Config, Mode models.ModeType) {
	var GenerateTimer, timer, QueryOptimizationTimer *time.Timer
	defer utils.HandlePanic(configuration, logger)

	if (Mode.Name == "Configurations" && Mode.Type != "default") || Mode.Name == "Event" || Mode.Name == "TaskSet" {
		GenerateTimer = time.NewTimer(1 * time.Second)
		timer = time.NewTimer(1 * time.Hour)
		QueryOptimizationTimer = time.NewTimer(1 * time.Hour)
	} else {
		GenerateTimer = time.NewTimer(configuration.GenerateConfigPeriod * time.Second)
		timer = time.NewTimer(1 * time.Second)
		QueryOptimizationTimer = time.NewTimer(1 * time.Minute)
	}
	QueryOptimizationCollectSqlText := time.NewTimer(1 * time.Second)
	models.SqlText = make(map[string]map[string]string)
	models.SqlTextMutex = sync.RWMutex{}

	if !configuration.QueryOptimization {
		QueryOptimizationCollectSqlText.Stop()
	}
	terminator := makeTerminateChannel()

loop:
	for {
		select {
		case <-terminator:
			logger.Info("Exiting")
			break loop
		case <-timer.C:
			logger.Info("* Starting to collect metrics...")
			timer.Reset(configuration.MetricsPeriod * time.Second)
			go func() {
				defer utils.HandlePanic(configuration, logger)
				metrics := utils.CollectMetrics(append(gatherers, gatherers_metrics...), logger, configuration)
				if metrics != nil {
					metrics.DB.Metrics.CountEnableEventsStatementsConsumers = utils.EnableEventsStatementsConsumers(configuration, logger, metrics.DB.Metrics.Status["Uptime"].(string))
					task := utils.ProcessRepeaters(metrics, repeaters, configuration, logger, models.ModeType{Name: "Metrics", Type: ""})
					if task == "Task" {
						logger.Info("* A task received by the agent...")
						f := tasks.ProcessTaskFunc(metrics, repeaters, gatherers, logger, configuration)
						time.AfterFunc(5*time.Second, f)
					}
				}
				logger.Info("* MySQL Metrics are saved...")
			}()
		case <-GenerateTimer.C:
			logger.Info("* Starting to collect metrics...")
			GenerateTimer.Reset(configuration.GenerateConfigPeriod * time.Second)
			go func() {
				var metrics *models.Metrics
				logger.Info("* Collecting metrics...")
				defer utils.HandlePanic(configuration, logger)
				if Mode.Name == "TaskSet" && Mode.Type == "queries_optimization" {
					metrics = utils.CollectMetrics(append(gatherers, gatherers_query_optimization...), logger, configuration)
				} else {
					metrics = utils.CollectMetrics(append(gatherers, gatherers_configuration...), logger, configuration)
				}
				if metrics != nil {
					metrics.DB.Metrics.CountEnableEventsStatementsConsumers = utils.EnableEventsStatementsConsumers(configuration, logger, "0")
					logger.Info("* Sending metrics to the Releem Cloud Platform...")
					utils.ProcessRepeaters(metrics, repeaters, configuration, logger, Mode)
					if Mode.Name == "Configurations" {
						logger.Info("* The recommended MySQL configuration has been downloaded to: ", configuration.GetReleemConfDir())
					}
				}
				if (Mode.Name == "Configurations" && Mode.Type != "default") || Mode.Name == "Event" || Mode.Name == "TaskSet" {
					logger.Info("Exiting")
					os.Exit(0)
				}
				logger.Info("* MySQL Metrics are saved...")
			}()
		case <-QueryOptimizationTimer.C:
			logger.Info("* Starting to collect MySQL metrics for Query Analytics...")
			QueryOptimizationTimer.Reset(configuration.QueryOptimizationPeriod * time.Second)
			go func() {
				defer utils.HandlePanic(configuration, logger)
				metrics := utils.CollectMetrics(append(gatherers, gatherers_query_optimization...), logger, configuration)
				if metrics != nil {
					utils.ProcessRepeaters(metrics, repeaters, configuration, logger, models.ModeType{Name: "Metrics", Type: "QueryOptimization"})
				}
				logger.Info("* MySQL metrics for Query Analytics are saved...")
			}()
		case <-QueryOptimizationCollectSqlText.C:
			QueryOptimizationCollectSqlText.Reset(configuration.QueryOptimizationCollectSqlTextPeriod * time.Second)
			go func() {
				defer utils.HandlePanic(configuration, logger)
				Ready = false
				rows, err := models.DB.Query("SELECT t2.`CURRENT_SCHEMA`, t2.`DIGEST`, t2.`SQL_TEXT` FROM (SELECT `CURRENT_SCHEMA`, `DIGEST`, MAX(`TIMER_START`) AS MAX_TIMER_START FROM `performance_schema`.`events_statements_history` WHERE `DIGEST` IS NOT NULL AND `CURRENT_SCHEMA` IS NOT NULL GROUP BY `CURRENT_SCHEMA`, `DIGEST` ) t1 JOIN `performance_schema`.`events_statements_history` t2 ON t2.`TIMER_START`=t1.`MAX_TIMER_START` AND t2.`CURRENT_SCHEMA`=t1.`CURRENT_SCHEMA` AND t2.`DIGEST`=t1.`DIGEST`")
				if err != nil {
					logger.Error(err)
				} else {
					defer rows.Close()

					// Batch collect data to reduce mutex contention
					tempData := make(map[string]map[string]string)
					var schema, digest, sqlText string

					for rows.Next() {
						err := rows.Scan(&schema, &digest, &sqlText)
						if err != nil {
							logger.Error(err)
							continue
						}

						if tempData[schema] == nil {
							tempData[schema] = make(map[string]string)
						}
						tempData[schema][digest] = sqlText
					}

					// Single mutex lock for batch update
					if len(tempData) > 0 {
						models.SqlTextMutex.Lock()
						for schema, digestMap := range tempData {
							if models.SqlText[schema] == nil {
								models.SqlText[schema] = make(map[string]string)
							}
							for digest, sqlText := range digestMap {
								models.SqlText[schema][digest] = sqlText
							}
						}
						models.SqlTextMutex.Unlock()
					}
				}

			}()
		}
	}
}
