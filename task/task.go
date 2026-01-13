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

func ProcessTaskFunc(metrics *models.Metrics, repeaters models.MetricsRepeater, gatherers []models.MetricsGatherer, logger logging.Logger, configuration *config.Config) func() {
	return func() {
		ProcessTask(metrics, repeaters, gatherers, logger, configuration)
	}
}

func ProcessTask(metrics *models.Metrics, repeaters models.MetricsRepeater, gatherers []models.MetricsGatherer, logger logging.Logger, configuration *config.Config) {
	defer utils.HandlePanic(configuration, logger)
	taskStruct := models.Task{}
	//metrics := collectMetrics(gatherers, logger)
	var task_output string

	task := utils.ProcessRepeaters(metrics, repeaters, configuration, logger, models.ModeType{Name: "Task", Type: "Get"})
	if task == nil {
		return
	}
	err := json.Unmarshal([]byte(task.(string)), &taskStruct)
	if err != nil {
		logger.Error("Failed to parse task description JSON: ", err)
		return
	}
	logger.Info(task)
	logger.Info(taskStruct)

	taskStruct.Status = 3
	taskStruct.Output = ""
	metrics.ReleemAgent.Tasks = taskStruct
	utils.ProcessRepeaters(metrics, repeaters, configuration, logger, models.ModeType{Name: "Task", Type: "Status"})
	logger.Info(" * Task with id - ", taskStruct.ID, " and type id - ", taskStruct.TypeID, " is being started...")

	switch taskStruct.TypeID {
	case 0:
		taskStruct.ExitCode, taskStruct.Status, task_output = execCmd(configuration.ReleemDir+"/mysqlconfigurer.sh -a", []string{"RELEEM_RESTART_SERVICE=1"}, logger)
		taskStruct.Output = taskStruct.Output + task_output

		if taskStruct.ExitCode == 7 {
			var rollback_exit_code int
			rollback_exit_code, _, task_output = execCmd(configuration.ReleemDir+"/mysqlconfigurer.sh -r", []string{"RELEEM_RESTART_SERVICE=1"}, logger)
			taskStruct.Output = taskStruct.Output + task_output
			logger.Info(" * Task rollbacked with code ", rollback_exit_code)
		}

	case 1:
		taskStruct.ExitCode, taskStruct.Status, task_output = execCmd(configuration.ReleemDir+"/releem-agent -f", []string{}, logger)
		taskStruct.Output = taskStruct.Output + task_output
	case 2:
		taskStruct.ExitCode, taskStruct.Status, task_output = execCmd(configuration.ReleemDir+"/mysqlconfigurer.sh -u", []string{}, logger)
		taskStruct.Output = taskStruct.Output + task_output
	case 3:
		taskStruct.ExitCode, taskStruct.Status, task_output = execCmd(configuration.ReleemDir+"/releem-agent --task=queries_optimization", []string{}, logger)
		taskStruct.Output = taskStruct.Output + task_output
	case 4:
		switch configuration.InstanceType {
		case "aws/rds":
			taskStruct.ExitCode, taskStruct.Status, task_output = ApplyConfAwsRds(repeaters, gatherers, logger, configuration, types.ApplyMethodImmediate)
			taskStruct.Output = taskStruct.Output + task_output
			if taskStruct.ExitCode == 0 {
				taskStruct.ExitCode, taskStruct.Status, task_output = ApplyConfAwsRds(repeaters, gatherers, logger, configuration, types.ApplyMethodPendingReboot)
				taskStruct.Output = taskStruct.Output + task_output
			}
		case "gcp/cloudsql":
			taskStruct.ExitCode, taskStruct.Status, task_output = ApplyConfGcpCloudSQL(repeaters, gatherers, logger, configuration)
			taskStruct.Output = taskStruct.Output + task_output

		default:
			switch runtime.GOOS {
			case "windows":
				taskStruct.ExitCode = 0
				taskStruct.Status = 1
				taskStruct.Output = taskStruct.Output + "Windows is not supported apply configuration.\n"
			default: // для Linux и других UNIX-подобных систем
				taskStruct.ExitCode, taskStruct.Status, task_output = execCmd(configuration.ReleemDir+"/mysqlconfigurer.sh -s automatic", []string{"RELEEM_RESTART_SERVICE=0"}, logger)
				taskStruct.Output = taskStruct.Output + task_output
				if taskStruct.ExitCode == 7 {
					var rollback_exit_code int
					rollback_exit_code, _, task_output = execCmd(configuration.ReleemDir+"/mysqlconfigurer.sh -r", []string{"RELEEM_RESTART_SERVICE=1"}, logger)
					taskStruct.Output = taskStruct.Output + task_output
					logger.Info(" * Task rollbacked with code ", rollback_exit_code)
				}
			}

			if taskStruct.ExitCode == 0 {
				taskStruct.ExitCode, taskStruct.Status, task_output = ApplyConfLocal(metrics, repeaters, gatherers, logger, configuration)
				taskStruct.Output = taskStruct.Output + task_output
			}
		}

	case 5:
		switch configuration.InstanceType {
		case "aws/rds":
			taskStruct.ExitCode, taskStruct.Status, task_output = ApplyConfAwsRds(repeaters, gatherers, logger, configuration, types.ApplyMethodPendingReboot)
			taskStruct.Output = taskStruct.Output + task_output
		case "gcp/cloudsql":
			taskStruct.ExitCode, taskStruct.Status, task_output = ApplyConfGcpCloudSQL(repeaters, gatherers, logger, configuration)
			taskStruct.Output = taskStruct.Output + task_output

		default:
			switch runtime.GOOS {
			case "windows":
				taskStruct.ExitCode = 0
				taskStruct.Status = 1
				taskStruct.Output = taskStruct.Output + "Windows is not supported apply configuration.\n"
			default: // для Linux и других UNIX-подобных систем
				taskStruct.ExitCode, taskStruct.Status, task_output = execCmd(configuration.ReleemDir+"/mysqlconfigurer.sh -s automatic", []string{"RELEEM_RESTART_SERVICE=1"}, logger)
				taskStruct.Output = taskStruct.Output + task_output
				if taskStruct.ExitCode == 7 {
					var rollback_exit_code int
					rollback_exit_code, _, task_output = execCmd(configuration.ReleemDir+"/mysqlconfigurer.sh -r", []string{"RELEEM_RESTART_SERVICE=1"}, logger)
					taskStruct.Output = taskStruct.Output + task_output
					logger.Info(" * Task rollbacked with code ", rollback_exit_code)
				}
			}
		}

	case 7:
		taskStruct.ExitCode, taskStruct.Status, task_output = ProcessQueryExplainTask(
			taskStruct.Description, logger, configuration, metrics)
		taskStruct.Output = taskStruct.Output + task_output
		utils.ProcessRepeaters(metrics, repeaters, configuration, logger, models.ModeType{Name: "Task", Type: "queries_optimization"})

	}
	time.Sleep(10 * time.Second)
	metrics = utils.CollectMetrics(gatherers, logger, configuration)
	logger.Info(" * Task with id - ", taskStruct.ID, " and type id - ", taskStruct.TypeID, " completed with code ", taskStruct.ExitCode)
	metrics.ReleemAgent.Tasks = taskStruct
	utils.ProcessRepeaters(metrics, repeaters, configuration, logger, models.ModeType{Name: "Task", Type: "Status"})

}
