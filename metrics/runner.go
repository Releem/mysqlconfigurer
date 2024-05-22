package metrics

import (
	"bytes"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
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

func RunWorker(gatherers []MetricsGatherer, gatherers_configuration []MetricsGatherer, repeaters map[string]MetricsRepeater, logger logging.Logger,
	configuration *config.Config, configFile string, Mode Mode) {
	var GenerateTimer, timer *time.Timer
	defer HandlePanic(configuration, logger)
	if logger == nil {
		if configuration.Debug {
			logger = logging.NewSimpleDebugLogger("Worker")
		} else {
			logger = logging.NewSimpleLogger("Worker")
		}
	}

	logger.Debug(configuration)
	if (Mode.Name == "Configurations" && Mode.ModeType != "default") || Mode.Name == "Events" || Mode.Name == "Task" {
		GenerateTimer = time.NewTimer(0 * time.Second)
		timer = time.NewTimer(3600 * time.Second)
	} else {
		GenerateTimer = time.NewTimer(configuration.GenerateConfigSeconds * time.Second)
		timer = time.NewTimer(1 * time.Second)
	}

	terminator := makeTerminateChannel()
	for {
		select {
		case <-terminator:
			logger.Info("Exiting")
			os.Exit(0)
		case <-timer.C:
			timer.Reset(configuration.TimePeriodSeconds * time.Second)
			go func() {
				defer HandlePanic(configuration, logger)
				Ready = false
				metrics := collectMetrics(gatherers, logger)
				if Ready {
					task := processRepeaters(metrics, repeaters["Metrics"], configuration, logger)
					if task == "Task" {
						logger.Println(" * A task has been found for the agent...")
						f := processTaskFunc(metrics, repeaters, logger, configuration)
						time.AfterFunc(5*time.Second, f)
					}
				}
			}()
		case <-GenerateTimer.C:
			GenerateTimer.Reset(configuration.GenerateConfigSeconds * time.Second)
			go func() {
				defer HandlePanic(configuration, logger)
				Ready = false
				logger.Println(" * Collecting metrics to recommend a config...")
				metrics := collectMetrics(append(gatherers, gatherers_configuration...), logger)
				if Ready {
					processRepeaters(metrics, repeaters[Mode.Name], configuration, logger)
				}
				if (Mode.Name == "Configurations" && Mode.ModeType != "default") || Mode.Name == "Events" || Mode.Name == "Task" {
					os.Exit(0)
				}
			}()
		}
	}
}

func processTaskFunc(metrics Metrics, repeaters map[string]MetricsRepeater, logger logging.Logger, configuration *config.Config) func() {
	return func() {
		processTask(metrics, repeaters, logger, configuration)
	}
}

func processTask(metrics Metrics, repeaters map[string]MetricsRepeater, logger logging.Logger, configuration *config.Config) {
	defer HandlePanic(configuration, logger)
	output := make(MetricGroupValue)
	//metrics := collectMetrics(gatherers, logger)
	logger.Println(metrics)
	logger.Println(repeaters["TaskGet"])

	task := processRepeaters(metrics, repeaters["TaskGet"], configuration, logger)
	if task.(Task).TaskTypeID == nil {
		return
	}

	TaskTypeID := *task.(Task).TaskTypeID
	TaskID := *task.(Task).TaskID
	var stdout, stderr bytes.Buffer

	output["task_id"] = TaskID
	output["task_type_id"] = TaskTypeID
	output["task_status"] = 3
	metrics.ReleemAgent.Tasks = output
	processRepeaters(metrics, repeaters["TaskStatus"], configuration, logger)
	logger.Println(" * Task with id -", TaskID, "and type id -", TaskTypeID, "is being started...")

	if TaskTypeID == 0 {
		output = execCmd(configuration.ReleemDir+"/mysqlconfigurer.sh -a", logger)
		task_output := output["task_output"].(string)

		if output["task_exit_code"] == 7 {
			var rollback_exit_code int
			cmd := exec.Command(configuration.ReleemDir+"/mysqlconfigurer.sh", "-r")
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			cmd.Env = append(cmd.Environ(), "RELEEM_RESTART_SERVICE=1")
			err := cmd.Run()
			if err != nil {
				output["task_output"] = task_output + err.Error()
				logger.Error(err)
				if exiterr, ok := err.(*exec.ExitError); ok {
					rollback_exit_code = exiterr.ExitCode()
				} else {
					rollback_exit_code = 999
				}
			} else {
				rollback_exit_code = 0
			}
			output["task_output"] = task_output + stdout.String() + stderr.String()
			logger.Println(" * Task rollbacked with code", rollback_exit_code)
		}

	} else if TaskTypeID == 1 {
		output = execCmd(configuration.ReleemDir+"/releem-agent -f", logger)
	} else if TaskTypeID == 2 {
		output = execCmd(configuration.ReleemDir+"/mysqlconfigurer.sh -u", logger)
	} else if TaskTypeID == 3 {
		output = execCmd(configuration.ReleemDir+"/releem-agent --task=collect_queries", logger)
	} else if TaskTypeID == 4 {
		task_output := ""

		// if configuration.InstanceType != "aws" {
		// 	output = execCmd(configuration.ReleemDir+"/mysqlconfigurer.sh -a", logger)
		// 	task_output = task_output + output["task_output"].(string)

		// }

		recommend_var := processRepeaters(metrics, repeaters["ConfigGet"], configuration, logger)
		need_restart := false
		need_privileges := false
		need_flush := false
		flush_queries := []string{"flush status", "flush statistic"}
		for _, variable := range recommend_var.(MetricGroupValue) {
			if recommend_var.(MetricGroupValue)[variable.(string)] != metrics.DB.Conf.Variables[variable.(string)] {
				_, err := config.DB.Exec("set global ? = ?", variable, recommend_var.(MetricGroupValue)[variable.(string)])
				if err != nil {
					log.Fatalf("not execute query %s", err)
					task_output = task_output + err.Error()
					if strings.Contains(err.Error(), "read-only") {
						need_restart = true
					} else if strings.Contains(err.Error(), "denied") {
						need_privileges = true
						break
					} else {
						if exiterr, ok := err.(*exec.ExitError); ok {
							output["task_exit_code"] = exiterr.ExitCode()
						} else {
							output["task_exit_code"] = 999
						}
						output["task_status"] = 4
						break
					}
				} else {
					need_flush = true
				}
			}
		}

		logger.Println(need_flush)
		logger.Println(need_restart)
		logger.Println(need_privileges)

		if need_flush {
			for _, query := range flush_queries {
				_, err := config.DB.Exec(query)
				if err != nil {
					task_output = task_output + err.Error()
					logger.Error(err)
					if exiterr, ok := err.(*exec.ExitError); ok {
						output["task_exit_code"] = exiterr.ExitCode()
					} else {
						output["task_exit_code"] = 999
					}
				} else {
					output["task_exit_code"] = 0
				}
			}
		}
		if need_privileges {
			output["task_exit_code"] = 0
			output["task_status"] = 11
		}
		if need_restart {
			output["task_exit_code"] = 0
			output["task_status"] = 22
		}

		output["task_output"] = task_output + stderr.String()

	}
	logger.Println(" * Task with id -", TaskID, "and type id -", TaskTypeID, "completed with code", output["task_exit_code"])

	metrics.ReleemAgent.Tasks = output
	logger.Debug(output)
	processRepeaters(metrics, repeaters["TaskStatus"], configuration, logger)

}

func execCmd(cmd_path string, logger logging.Logger) MetricGroupValue {
	output := make(MetricGroupValue)
	var stdout, stderr bytes.Buffer

	cmd := exec.Command("sh", "-c", cmd_path)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	task_output := ""
	if err != nil {
		task_output = task_output + err.Error()
		logger.Error(err)
		if exiterr, ok := err.(*exec.ExitError); ok {
			output["task_exit_code"] = exiterr.ExitCode()
		} else {
			output["task_exit_code"] = 999
		}
		output["task_status"] = 4
	} else {
		output["task_exit_code"] = 0
		output["task_status"] = 1
	}
	output["task_output"] = task_output + stderr.String()
	return output
}
func processRepeaters(metrics Metrics, repeaters MetricsRepeater,
	configuration *config.Config, logger logging.Logger) interface{} {
	defer HandlePanic(configuration, logger)

	result, err := repeaters.ProcessMetrics(configuration, metrics)
	if err != nil {
		logger.PrintError("Repeater failed", err)
	}
	return result
}

func collectMetrics(gatherers []MetricsGatherer, logger logging.Logger) Metrics {
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
