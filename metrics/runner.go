package metrics

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Releem/mysqlconfigurer/config"
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

func RunWorker(gatherers []MetricsGatherer, gatherers_configuration []MetricsGatherer, gatherers_query_optimization []MetricsGatherer, repeaters MetricsRepeater, logger logging.Logger,
	configuration *config.Config, configFile string, Mode ModeT) {
	var GenerateTimer, timer, QueryOptimizationTimer *time.Timer
	defer HandlePanic(configuration, logger)
	if logger == nil {
		if configuration.Debug {
			logger = logging.NewSimpleDebugLogger("Worker")
		} else {
			logger = logging.NewSimpleLogger("Worker")
		}
	}

	if (Mode.Name == "Configurations" && Mode.ModeType != "default") || Mode.Name == "Event" || Mode.Name == "TaskSet" {
		GenerateTimer = time.NewTimer(0 * time.Second)
		timer = time.NewTimer(3600 * time.Second)
	} else {
		GenerateTimer = time.NewTimer(configuration.GenerateConfigPeriod * time.Second)
		timer = time.NewTimer(1 * time.Second)
	}
	QueryOptimizationTimer = time.NewTimer(60 * time.Second)
	QueryOptimizationCollectSqlText := time.NewTimer(1 * time.Second)
	config.SqlText = make(map[string]map[string]string)
	config.SqlTextMutex = sync.RWMutex{}

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
				defer HandlePanic(configuration, logger)
				Ready = false
				metrics := collectMetrics(gatherers, logger, configuration)
				if Ready {
					task := processRepeaters(metrics, repeaters, configuration, logger, ModeT{Name: "Metrics", ModeType: ""})
					if task == "Task" {
						logger.Println(" * A task has been found for the agent...")
						f := processTaskFunc(metrics, repeaters, gatherers, logger, configuration)
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
				var metrics Metrics
				logger.Println(" * Collecting metrics to recommend a config...")
				defer HandlePanic(configuration, logger)
				Ready = false
				if Mode.Name == "TaskSet" && Mode.ModeType == "queries_optimization" {
					metrics = collectMetrics(append(gatherers, gatherers_query_optimization...), logger, configuration)
				} else {
					metrics = collectMetrics(append(gatherers, gatherers_configuration...), logger, configuration)
				}
				if Ready {
					logger.Println(" * Sending metrics to Releem Cloud Platform...")
					processRepeaters(metrics, repeaters, configuration, logger, Mode)
					if Mode.Name == "Configurations" {
						logger.Println("Recommended MySQL configuration downloaded to ", configuration.GetReleemConfDir())
					}
				}
				if (Mode.Name == "Configurations" && Mode.ModeType != "default") || Mode.Name == "Event" || Mode.Name == "TaskSet" {
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
				defer HandlePanic(configuration, logger)
				Ready = false
				logger.Println("QueryOptimization")
				metrics := collectMetrics(append(gatherers, gatherers_query_optimization...), logger, configuration)
				if Ready {
					processRepeaters(metrics, repeaters, configuration, logger, ModeT{Name: "Metrics", ModeType: "QueryOptimization"})
				}
				logger.Println("Saved a queries...")
			}()
		case <-QueryOptimizationCollectSqlText.C:
			QueryOptimizationCollectSqlText.Reset(configuration.QueryOptimizationCollectSqlTextPeriod * time.Second)
			go func() {
				defer HandlePanic(configuration, logger)
				Ready = false
				var SqlText_elem SqlTextType
				rows, err := config.DB.Query("SELECT CURRENT_SCHEMA, DIGEST, SQL_TEXT FROM performance_schema.events_statements_history WHERE DIGEST IS NOT NULL AND CURRENT_SCHEMA IS NOT NULL GROUP BY current_schema, digest")
				if err != nil {
					logger.Error(err)
				} else {
					for rows.Next() {
						err := rows.Scan(&SqlText_elem.CURRENT_SCHEMA, &SqlText_elem.DIGEST, &SqlText_elem.SQL_TEXT)
						if err != nil {
							logger.Error(err)
						} else {
							config.SqlTextMutex.Lock()
							if config.SqlText[SqlText_elem.CURRENT_SCHEMA] == nil {
								config.SqlText[SqlText_elem.CURRENT_SCHEMA] = make(map[string]string)
							}
							config.SqlText[SqlText_elem.CURRENT_SCHEMA][SqlText_elem.DIGEST] = SqlText_elem.SQL_TEXT
							config.SqlTextMutex.Unlock()
						}
					}
				}

			}()
		}
	}
}

func processTaskFunc(metrics Metrics, repeaters MetricsRepeater, gatherers []MetricsGatherer, logger logging.Logger, configuration *config.Config) func() {
	return func() {
		processTask(metrics, repeaters, gatherers, logger, configuration)
	}
}

func processTask(metrics Metrics, repeaters MetricsRepeater, gatherers []MetricsGatherer, logger logging.Logger, configuration *config.Config) {
	defer HandlePanic(configuration, logger)
	output := make(MetricGroupValue)
	//metrics := collectMetrics(gatherers, logger)
	var task_output string
	task := processRepeaters(metrics, repeaters, configuration, logger, ModeT{Name: "TaskGet", ModeType: ""})
	if task.(Task).TaskTypeID == nil {
		return
	}

	TaskTypeID := *task.(Task).TaskTypeID
	TaskID := *task.(Task).TaskID
	var stdout, stderr bytes.Buffer

	output["task_id"] = TaskID
	output["task_type_id"] = TaskTypeID
	output["task_status"] = 3
	output["task_output"] = ""

	metrics.ReleemAgent.Tasks = output
	processRepeaters(metrics, repeaters, configuration, logger, ModeT{Name: "TaskStatus", ModeType: ""})
	logger.Println(" * Task with id -", TaskID, "and type id -", TaskTypeID, "is being started...")

	if TaskTypeID == 0 {
		output["task_exit_code"], output["task_status"], task_output = execCmd(configuration.ReleemDir+"/mysqlconfigurer.sh -a", []string{"RELEEM_RESTART_SERVICE=1"}, logger)
		output["task_output"] = output["task_output"].(string) + task_output

		if output["task_exit_code"] == 7 {
			var rollback_exit_code int
			cmd := exec.Command(configuration.ReleemDir+"/mysqlconfigurer.sh", "-r")
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			cmd.Env = append(cmd.Environ(), "RELEEM_RESTART_SERVICE=1")
			err := cmd.Run()
			if err != nil {
				output["task_output"] = output["task_output"].(string) + err.Error()
				logger.Error(err)
				if exiterr, ok := err.(*exec.ExitError); ok {
					rollback_exit_code = exiterr.ExitCode()
				} else {
					rollback_exit_code = 999
				}
			} else {
				rollback_exit_code = 0
			}
			output["task_output"] = output["task_output"].(string) + stdout.String() + stderr.String()
			logger.Println(" * Task rollbacked with code", rollback_exit_code)
		}

	} else if TaskTypeID == 1 {
		output["task_exit_code"], output["task_status"], task_output = execCmd(configuration.ReleemDir+"/releem-agent -f", []string{}, logger)
		output["task_output"] = output["task_output"].(string) + task_output
	} else if TaskTypeID == 2 {
		output["task_exit_code"], output["task_status"], task_output = execCmd(configuration.ReleemDir+"/mysqlconfigurer.sh -u", []string{}, logger)
		output["task_output"] = output["task_output"].(string) + task_output
	} else if TaskTypeID == 3 {
		output["task_exit_code"], output["task_status"], task_output = execCmd(configuration.ReleemDir+"/releem-agent --task=queries_optimization", []string{}, logger)
		output["task_output"] = output["task_output"].(string) + task_output
	} else if TaskTypeID == 4 {
		if configuration.InstanceType != "aws" {
			output["task_exit_code"], output["task_status"], task_output = execCmd(configuration.ReleemDir+"/mysqlconfigurer.sh -s automatic", []string{"RELEEM_RESTART_SERVICE=0"}, logger)
			output["task_output"] = output["task_output"].(string) + task_output
		}
		if output["task_exit_code"] == 0 {
			result_data := MetricGroupValue{}
			// flush_queries := []string{"flush status", "flush statistic"}
			need_restart := false
			need_privileges := false
			need_flush := false
			error_exist := false

			recommend_var := processRepeaters(metrics, repeaters, configuration, logger, ModeT{Name: "Configurations", ModeType: "get-json"})
			err := json.Unmarshal([]byte(recommend_var.(string)), &result_data)
			if err != nil {
				logger.Error(err)
			}

			for key := range result_data {
				logger.Println(key, result_data[key], metrics.DB.Conf.Variables[key])

				if result_data[key] != metrics.DB.Conf.Variables[key] {
					query_set_var := "set global " + key + "=" + result_data[key].(string)
					_, err := config.DB.Exec(query_set_var)
					if err != nil {
						logger.Error(err)
						output["task_output"] = output["task_output"].(string) + err.Error()
						if strings.Contains(err.Error(), "is a read only variable") {
							need_restart = true
						} else if strings.Contains(err.Error(), "Access denied") {
							need_privileges = true
							break
						} else {
							error_exist = true
							break
						}
					} else {
						need_flush = true
					}
				}
			}
			logger.Println(need_flush, need_restart, need_privileges, error_exist)
			if error_exist {
				output["task_exit_code"] = 8
				output["task_status"] = 4
			} else {
				// if need_flush {
				// 	for _, query := range flush_queries {
				// 		_, err := config.DB.Exec(query)
				// 		if err != nil {
				// 			output["task_output"] = output["task_output"].(string) + err.Error()
				// 			logger.Error(err)
				// 			// if exiterr, ok := err.(*exec.ExitError); ok {
				// 			// 	output["task_exit_code"] = exiterr.ExitCode()
				// 			// } else {
				// 			// 	output["task_exit_code"] = 999
				// 			// }
				// 		}
				// 		// } else {
				// 		// 	output["task_exit_code"] = 0
				// 		// }
				// 	}
				// }
				if need_privileges {
					output["task_exit_code"] = 9
					output["task_status"] = 4
				} else if need_restart {
					output["task_exit_code"] = 10
				}
			}
			time.Sleep(10 * time.Second)
			metrics = collectMetrics(gatherers, logger, configuration)
		}
	} else if TaskTypeID == 5 {
		output["task_exit_code"], output["task_status"], task_output = execCmd(configuration.ReleemDir+"/mysqlconfigurer.sh -s automatic", []string{"RELEEM_RESTART_SERVICE=1"}, logger)
		output["task_output"] = output["task_output"].(string) + task_output

		if output["task_exit_code"] == 7 {
			var rollback_exit_code int
			cmd := exec.Command(configuration.ReleemDir+"/mysqlconfigurer.sh", "-r")
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			cmd.Env = append(cmd.Environ(), "RELEEM_RESTART_SERVICE=1")
			err := cmd.Run()
			if err != nil {
				output["task_output"] = output["task_output"].(string) + err.Error()
				logger.Error(err)
				if exiterr, ok := err.(*exec.ExitError); ok {
					rollback_exit_code = exiterr.ExitCode()
				} else {
					rollback_exit_code = 999
				}
			} else {
				rollback_exit_code = 0
			}
			output["task_output"] = output["task_output"].(string) + stdout.String() + stderr.String()
			logger.Println(" * Task rollbacked with code", rollback_exit_code)
		}
	}
	logger.Debug(output)
	logger.Println(" * Task with id -", TaskID, "and type id -", TaskTypeID, "completed with code", output["task_exit_code"])
	metrics.ReleemAgent.Tasks = output
	processRepeaters(metrics, repeaters, configuration, logger, ModeT{Name: "TaskStatus", ModeType: ""})

}

func execCmd(cmd_path string, environment []string, logger logging.Logger) (int, int, string) {
	var stdout, stderr bytes.Buffer
	var task_exit_code, task_status int
	cmd := exec.Command("sh", "-c", cmd_path)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	for _, env := range environment {
		cmd.Env = append(cmd.Environ(), env)
	}
	err := cmd.Run()
	task_output := ""
	if err != nil {
		task_output = task_output + err.Error()
		logger.Error(err)
		if exiterr, ok := err.(*exec.ExitError); ok {
			task_exit_code = exiterr.ExitCode()
		} else {
			task_exit_code = 999
		}
		task_status = 4
	} else {
		task_exit_code = 0
		task_status = 1
	}
	task_output = task_output + stdout.String() + stderr.String()
	return task_exit_code, task_status, task_output
}
func processRepeaters(metrics Metrics, repeaters MetricsRepeater,
	configuration *config.Config, logger logging.Logger, Mode ModeT) interface{} {
	defer HandlePanic(configuration, logger)

	result, err := repeaters.ProcessMetrics(configuration, metrics, Mode)
	if err != nil {
		logger.PrintError("Repeater failed", err)
	}
	return result
}

func collectMetrics(gatherers []MetricsGatherer, logger logging.Logger, configuration *config.Config) Metrics {
	defer HandlePanic(configuration, logger)
	var metrics Metrics
	for _, g := range gatherers {
		err := g.GetMetrics(&metrics)
		if err != nil {
			logger.Error("Problem getting metrics from gatherer")
			return Metrics{}
		}
	}
	Ready = true
	return metrics
}
