package task

import (
	"encoding/json"
	"runtime"
	"time"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"
	logging "github.com/google/logger"
)

func ProcessTaskFunc(repeaters models.MetricsRepeater, gatherers []models.MetricsGatherer, logger logging.Logger, configuration *config.Config) func() {
	return func() {
		ProcessTask(repeaters, gatherers, logger, configuration)
	}
}

func ProcessTask(repeaters models.MetricsRepeater, gatherers []models.MetricsGatherer, logger logging.Logger, configuration *config.Config) {
	defer utils.HandlePanic(configuration, logger)
	TaskStruct := models.Task{}
	metrics := utils.CollectMetrics(gatherers, logger, configuration)

	var task_output string

	RepeaterResponse := utils.ProcessRepeaters(metrics, repeaters, configuration, logger, models.ModeType{Name: "Task", Type: "Get"})
	if RepeaterResponse == "" {
		return
	}
	err := json.Unmarshal([]byte(RepeaterResponse), &TaskStruct)
	if err != nil {
		logger.Error("Failed to parse task description JSON: ", err)
		return
	}
	TaskStruct.Status = 3
	TaskStruct.Output = ""
	metrics.ReleemAgent.Tasks = TaskStruct
	utils.ProcessRepeaters(metrics, repeaters, configuration, logger, models.ModeType{Name: "Task", Type: "Status"})
	logger.Info(" * Task with id - ", TaskStruct.ID, " and type id - ", TaskStruct.TypeID, " is being started...")

	switch TaskStruct.TypeID {
	case 0:
		TaskStruct.ExitCode, TaskStruct.Status, task_output = execCmd(configuration.ReleemDir+"/mysqlconfigurer.sh -a", []string{"RELEEM_RESTART_SERVICE=1"}, logger)
		TaskStruct.Output = TaskStruct.Output + task_output

		if TaskStruct.ExitCode == 7 {
			var rollback_exit_code int
			rollback_exit_code, _, task_output = execCmd(configuration.ReleemDir+"/mysqlconfigurer.sh -r", []string{"RELEEM_RESTART_SERVICE=1"}, logger)
			TaskStruct.Output = TaskStruct.Output + task_output
			logger.Info(" * Task rollbacked with code ", rollback_exit_code)
		}

	case 1:
		TaskStruct.ExitCode, TaskStruct.Status, task_output = execCmd(configuration.ReleemDir+"/releem-agent -f", []string{}, logger)
		TaskStruct.Output = TaskStruct.Output + task_output
	case 2:
		TaskStruct.ExitCode, TaskStruct.Status, task_output = execCmd(configuration.ReleemDir+"/mysqlconfigurer.sh -u", []string{}, logger)
		TaskStruct.Output = TaskStruct.Output + task_output
	case 3:
		TaskStruct.ExitCode, TaskStruct.Status, task_output = execCmd(configuration.ReleemDir+"/releem-agent --task=queries_optimization", []string{}, logger)
		TaskStruct.Output = TaskStruct.Output + task_output
	case 4:
		switch configuration.InstanceType {
		case "aws/rds":
			TaskStruct.ExitCode, TaskStruct.Status, task_output = ApplyConfAwsRds(repeaters, gatherers, logger, configuration, types.ApplyMethodImmediate)
			TaskStruct.Output = TaskStruct.Output + task_output
			if TaskStruct.ExitCode == 0 {
				TaskStruct.ExitCode, TaskStruct.Status, task_output = ApplyConfAwsRds(repeaters, gatherers, logger, configuration, types.ApplyMethodPendingReboot)
				TaskStruct.Output = TaskStruct.Output + task_output
			}
		case "gcp/cloudsql":
			TaskStruct.ExitCode, TaskStruct.Status, task_output = ApplyConfGcpCloudSQL(repeaters, gatherers, logger, configuration)
			TaskStruct.Output = TaskStruct.Output + task_output

		default:
			switch runtime.GOOS {
			case "windows":
				TaskStruct.ExitCode = 0
				TaskStruct.Status = 1
				TaskStruct.Output = TaskStruct.Output + "Windows is not supported apply configuration.\n"
			default: // для Linux и других UNIX-подобных систем
				TaskStruct.ExitCode, TaskStruct.Status, task_output = execCmd(configuration.ReleemDir+"/mysqlconfigurer.sh -s automatic", []string{"RELEEM_RESTART_SERVICE=0"}, logger)
				TaskStruct.Output = TaskStruct.Output + task_output
				if TaskStruct.ExitCode == 7 {
					var rollback_exit_code int
					rollback_exit_code, _, task_output = execCmd(configuration.ReleemDir+"/mysqlconfigurer.sh -r", []string{"RELEEM_RESTART_SERVICE=1"}, logger)
					TaskStruct.Output = TaskStruct.Output + task_output
					logger.Info(" * Task rollbacked with code ", rollback_exit_code)
				}
			}

			if TaskStruct.ExitCode == 0 {
				TaskStruct.ExitCode, TaskStruct.Status, task_output = ApplyConfLocal(metrics, repeaters, gatherers, logger, configuration)
				TaskStruct.Output = TaskStruct.Output + task_output
			}
		}

	case 5:
		switch configuration.InstanceType {
		case "aws/rds":
			TaskStruct.ExitCode, TaskStruct.Status, task_output = ApplyConfAwsRds(repeaters, gatherers, logger, configuration, types.ApplyMethodPendingReboot)
			TaskStruct.Output = TaskStruct.Output + task_output
		case "gcp/cloudsql":
			TaskStruct.ExitCode, TaskStruct.Status, task_output = ApplyConfGcpCloudSQL(repeaters, gatherers, logger, configuration)
			TaskStruct.Output = TaskStruct.Output + task_output

		default:
			switch runtime.GOOS {
			case "windows":
				TaskStruct.ExitCode = 0
				TaskStruct.Status = 1
				TaskStruct.Output = TaskStruct.Output + "Windows is not supported apply configuration.\n"
			default: // для Linux и других UNIX-подобных систем
				TaskStruct.ExitCode, TaskStruct.Status, task_output = execCmd(configuration.ReleemDir+"/mysqlconfigurer.sh -s automatic", []string{"RELEEM_RESTART_SERVICE=1"}, logger)
				TaskStruct.Output = TaskStruct.Output + task_output
				if TaskStruct.ExitCode == 7 {
					var rollback_exit_code int
					rollback_exit_code, _, task_output = execCmd(configuration.ReleemDir+"/mysqlconfigurer.sh -r", []string{"RELEEM_RESTART_SERVICE=1"}, logger)
					TaskStruct.Output = TaskStruct.Output + task_output
					logger.Info(" * Task rollbacked with code ", rollback_exit_code)
				}
			}
		}

	case 7:
		TaskStruct.ExitCode, TaskStruct.Status, task_output = ProcessQueryExplainTask(
			TaskStruct.Details, logger, configuration, metrics)
		TaskStruct.Output = TaskStruct.Output + task_output
		RepeaterResponse := utils.ProcessRepeaters(metrics, repeaters, configuration, logger, models.ModeType{Name: "TaskByName", Type: "custom_queries_optimization"})
		mergedJSON, err := utils.MergeJSONStrings(TaskStruct.Details, RepeaterResponse, "query_optimization_result")
		if err != nil {
			logger.Error("Failed to merge task_details JSON: ", err)
		} else {
			TaskStruct.Details = mergedJSON
		}

	}

	time.Sleep(10 * time.Second)
	metrics = utils.CollectMetrics(gatherers, logger, configuration)
	logger.Info(" * Task with id - ", TaskStruct.ID, " and type id - ", TaskStruct.TypeID, " completed with code ", TaskStruct.ExitCode)
	metrics.ReleemAgent.Tasks = TaskStruct

	utils.ProcessRepeaters(metrics, repeaters, configuration, logger, models.ModeType{Name: "Task", Type: "Status"})
}
