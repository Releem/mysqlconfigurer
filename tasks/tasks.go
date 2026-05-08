package tasks

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
	var TaskStruct *models.Task
	var task_output string

	metrics := utils.CollectMetrics(gatherers, logger, configuration)
	RepeaterResponse := utils.ProcessRepeaters(metrics, repeaters, configuration, logger, models.ModeType{Name: "Task", Type: "Get"})
	logger.Infof("Task details: %s", RepeaterResponse)

	err := json.Unmarshal([]byte(RepeaterResponse), &TaskStruct)
	if err != nil {
		logger.Error("Failed to parse task description JSON: ", err)
		return
	}
	if TaskStruct == nil {
		return
	}

	metrics.ReleemAgent.Tasks = models.Task{ID: TaskStruct.ID, TypeID: TaskStruct.TypeID, Status: 3}
	utils.ProcessRepeaters(metrics, repeaters, configuration, logger, models.ModeType{Name: "Task", Type: "Status"})
	logger.Infof(" * Task with id - %d and type id - %d is being started...", TaskStruct.ID, TaskStruct.TypeID)

	switch TaskStruct.TypeID {
	case 0:
		TaskStruct.ExitCode, TaskStruct.Status, task_output = execTaskCommand(taskApplyManualCommand(runtime.GOOS, configuration.ReleemDir), logger)
		TaskStruct.Output = TaskStruct.Output + task_output

		if TaskStruct.ExitCode == 7 {
			var rollback_exit_code int
			rollback_exit_code, _, task_output = execTaskCommand(taskRollbackCommand(runtime.GOOS, configuration.ReleemDir), logger)
			TaskStruct.Output = TaskStruct.Output + task_output
			logger.Info(" * Task rollbacked with code ", rollback_exit_code)
		}

	case 1:
		TaskStruct.ExitCode, TaskStruct.Status, task_output = execTaskCommand(taskGenerateConfigCommand(runtime.GOOS, configuration.ReleemDir), logger)
		TaskStruct.Output = TaskStruct.Output + task_output
	case 2:
		TaskStruct.ExitCode, TaskStruct.Status, task_output = execTaskCommand(taskUpdateCommand(runtime.GOOS, configuration.ReleemDir), logger)
		TaskStruct.Output = TaskStruct.Output + task_output
	case 3:
		TaskStruct.ExitCode, TaskStruct.Status, task_output = execTaskCommand(taskQueriesOptimizationCommand(runtime.GOOS, configuration.ReleemDir), logger)
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
		case "azure/mysql":
			TaskStruct.ExitCode, TaskStruct.Status, task_output = ApplyConfAzureMySQL(repeaters, gatherers, logger, configuration, false)
			TaskStruct.Output = TaskStruct.Output + task_output

		default:
			TaskStruct.ExitCode, TaskStruct.Status, task_output = execTaskCommand(taskApplyAutomaticCommand(runtime.GOOS, configuration.ReleemDir, false), logger)
			TaskStruct.Output = TaskStruct.Output + task_output
			if TaskStruct.ExitCode == 7 {
				var rollback_exit_code int
				rollback_exit_code, _, task_output = execTaskCommand(taskRollbackCommand(runtime.GOOS, configuration.ReleemDir), logger)
				TaskStruct.Output = TaskStruct.Output + task_output
				logger.Info(" * Task rollbacked with code ", rollback_exit_code)
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
		case "azure/mysql":
			TaskStruct.ExitCode, TaskStruct.Status, task_output = ApplyConfAzureMySQL(repeaters, gatherers, logger, configuration, true)
			TaskStruct.Output = TaskStruct.Output + task_output

		default:
			TaskStruct.ExitCode, TaskStruct.Status, task_output = execTaskCommand(taskApplyAutomaticCommand(runtime.GOOS, configuration.ReleemDir, true), logger)
			TaskStruct.Output = TaskStruct.Output + task_output
			if TaskStruct.ExitCode == 7 {
				var rollback_exit_code int
				rollback_exit_code, _, task_output = execTaskCommand(taskRollbackCommand(runtime.GOOS, configuration.ReleemDir), logger)
				TaskStruct.Output = TaskStruct.Output + task_output
				logger.Info(" * Task rollbacked with code ", rollback_exit_code)
			}
		}

	case 7:
		TaskStruct.ExitCode, TaskStruct.Status, TaskStruct.Output, TaskStruct.Error = ProcessQueryExplainTask(
			TaskStruct.Details, logger, configuration, metrics)
		if TaskStruct.ExitCode == 0 {
			metrics.ReleemAgent.Tasks = *TaskStruct
			RepeaterResponse := utils.ProcessRepeaters(metrics, repeaters, configuration, logger, models.ModeType{Name: "TaskByName", Type: "custom_queries_optimization"})

			mergedJSON, err := utils.MergeJSONStrings(TaskStruct.Details, RepeaterResponse, "custom_queries_optimization_response")
			if err != nil {
				logger.Error("Failed to merge task_details JSON: ", err)
			} else {
				TaskStruct.Details = mergedJSON
			}
		}
	default:
		TaskStruct.ExitCode = 4 // unknown task type
		TaskStruct.Status = 4
	}

	time.Sleep(10 * time.Second)
	metrics = utils.CollectMetrics(gatherers, logger, configuration)
	logger.Infof(" * Task with id - %d and type id - %d completed with code %d", TaskStruct.ID, TaskStruct.TypeID, TaskStruct.ExitCode)

	metrics.ReleemAgent.Tasks = *TaskStruct
	utils.ProcessRepeaters(metrics, repeaters, configuration, logger, models.ModeType{Name: "Task", Type: "Status"})
}
