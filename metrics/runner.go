package metrics

import (
	"bytes"
	"os"
	"os/exec"
	"os/signal"
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
	var GenerateTimer *time.Timer
	defer HandlePanic(configuration, logger)
	if logger == nil {
		if configuration.Debug {
			logger = logging.NewSimpleDebugLogger("Worker")
		} else {
			logger = logging.NewSimpleLogger("Worker")
		}
	}

	logger.Debug(configuration)
	timer := time.NewTimer(1 * time.Second)
	configTimer := time.NewTimer(configuration.ReadConfigSeconds * time.Second)
	if (Mode.Name == "Configurations" && Mode.ModeType != "default") || Mode.Name == "Events" || Mode.Name == "Task" {
		GenerateTimer = time.NewTimer(0 * time.Second)

	} else {
		GenerateTimer = time.NewTimer(configuration.GenerateConfigSeconds * time.Second)
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
		case <-configTimer.C:
			configTimer.Reset(configuration.ReadConfigSeconds * time.Second)
			if newConfig, err := config.LoadConfig(configFile, logger); err != nil {
				logger.PrintError("Error reading config", err)
			} else {
				configuration = newConfig
				logger.Debug("LOADED NEW CONFIG", "APIKEY", configuration.GetApiKey())
			}

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
	task := processRepeaters(metrics, repeaters["Tasks"], configuration, logger)
	if task.(Task).TaskTypeID != nil {
		TaskTypeID := *task.(Task).TaskTypeID
		TaskID := *task.(Task).TaskID
		var stdout, stderr bytes.Buffer

		output["task_id"] = TaskID
		output["task_type_id"] = TaskTypeID
		output["task_status"] = 3
		metrics.ReleemAgent.Tasks = output
		logger.Println(" * Task with id -", TaskID, "and type id -", TaskTypeID, "is being started...")
		if TaskTypeID == 0 {
			cmd := exec.Command(configuration.ReleemDir+"/mysqlconfigurer.sh", "-a")
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			cmd.Env = append(cmd.Environ(), "RELEEM_RESTART_SERVICE=1")
			processRepeaters(metrics, repeaters["TaskStatus"], configuration, logger)
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
			output["task_output"] = task_output + stdout.String() + stderr.String()

			if output["task_exit_code"] == 7 {
				var rollback_exit_code int
				cmd := exec.Command(configuration.ReleemDir+"/mysqlconfigurer.sh", "-r")
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr
				cmd.Env = append(cmd.Environ(), "RELEEM_RESTART_SERVICE=1")
				err := cmd.Run()
				if err != nil {
					task_output = task_output + err.Error()
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
			logger.Println(" * Task with id -", TaskID, "and type id -", TaskTypeID, "completed with code", output["task_exit_code"])

			metrics.ReleemAgent.Tasks = output
			logger.Debug(output)
			processRepeaters(metrics, repeaters["TaskStatus"], configuration, logger)
		} else if TaskTypeID == 1 {
			cmd := exec.Command(configuration.ReleemDir+"/releem-agent", "-f")
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			processRepeaters(metrics, repeaters["TaskStatus"], configuration, logger)
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
			logger.Println(" * Task with id -", TaskID, "and type id -", TaskTypeID, "completed with code", output["task_exit_code"])

			metrics.ReleemAgent.Tasks = output
			logger.Debug(output)
			processRepeaters(metrics, repeaters["TaskStatus"], configuration, logger)
		}
	}
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
