package tasks

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/task-automator/pkg/phase2"
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

	case 6:
		logger.Info("Task status is ", TaskStruct.Status)
		TaskStruct.ExitCode, TaskStruct.Status, TaskStruct.Output, TaskStruct.Error = ApplySchemaChanges(logger, configuration, TaskStruct.Details, TaskStruct.Status)

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

func ApplySchemaChanges(logger logging.Logger, configuration *config.Config, taskdetails string, current_task_status int) (int, int, string, string) {
	var task_exit_code, task_status int = 0, 0
	var task_output string
	var task_error string

	type schemaChangeAnalysisResults struct {
		SchemaName             string      `json:"schema_name"`
		TableName              string      `json:"table_name"`
		SyntaxValid            bool        `json:"syntax_valid"`
		SyntaxError            interface{} `json:"syntax_error"`
		StorageEngine          string      `json:"storage_engine"`
		OKOnlineDDL            bool        `json:"ok_online_ddl"`
		OKPTOSC                bool        `json:"ok_pt_osc"`
		OKOnlinePhysicalBackup bool        `json:"ok_online_physical_backup"`
	}

	type schemaChangeTaskDetail struct {
		SchemaName      string                      `json:"schema_name"`
		DDLStatement    string                      `json:"ddl_statement"`
		AnalysisResults schemaChangeAnalysisResults `json:"analysis_results"`
	}

	parseTaskDetails := func() ([]schemaChangeTaskDetail, error) {
		var details []schemaChangeTaskDetail
		if err := json.Unmarshal([]byte(taskdetails), &details); err != nil {
			return nil, fmt.Errorf("taskdetails JSON must be an array of objects with `schema_name`, `ddl_statement`, and `analysis_results`")
		}
		if len(details) == 0 {
			return nil, fmt.Errorf("taskdetails array is empty")
		}

		for i, item := range details {
			if strings.TrimSpace(item.SchemaName) == "" {
				return nil, fmt.Errorf("taskdetails[%d].schema_name is required", i)
			}

			ddl := strings.TrimSpace(item.DDLStatement)
			if ddl == "" {
				return nil, fmt.Errorf("taskdetails[%d].ddl_statement is required", i)
			}

			if strings.TrimSpace(item.AnalysisResults.SchemaName) == "" {
				return nil, fmt.Errorf("taskdetails[%d].analysis_results.schema_name is required", i)
			}
			if strings.TrimSpace(item.AnalysisResults.TableName) == "" {
				return nil, fmt.Errorf("taskdetails[%d].analysis_results.table_name is required", i)
			}
		}

		return details, nil
	}

	if current_task_status == 1 {
		details, err := parseTaskDetails()
		if err != nil {
			logger.Error(err)
			task_exit_code = 2
			task_status = 4
			task_error = err.Error()
			task_output = task_output + err.Error()
			return task_exit_code, task_status, task_output, task_error
		}
		if len(details) == 0 {
			logger.Error("task_details list is empty")
			task_exit_code = 3
			task_status = 4
			task_error = "task_details list is empty"
			task_output = task_output + "Invalid task_details: empty schema change list."
			return task_exit_code, task_status, task_output, task_error
		}

		logger.Info("* Executing schema changes...")
		executor := phase2.NewExecutor(models.DB)
		executed := 0
		for i, item := range details {
			analysis := item.AnalysisResults
			statement := strings.TrimSpace(item.DDLStatement)
			tableName := analysis.SchemaName + "." + analysis.TableName

			if !analysis.SyntaxValid {
				errMsg := "syntax validation failed"
				if analysis.SyntaxError != nil {
					errMsg = fmt.Sprintf("syntax validation failed: %v", analysis.SyntaxError)
				}
				logger.Error("* Statement ", i, " - ", errMsg)
				task_output += fmt.Sprintf("Statement %d skipped: %s\n", i, errMsg)
				task_exit_code = 1
				task_status = 4
				continue
			}

			usePTOSC := !analysis.OKOnlineDDL && analysis.OKPTOSC
			if !analysis.OKOnlineDDL && !analysis.OKPTOSC {
				logger.Info("* Statement ", i, " - neither Online DDL nor pt-osc allowed by analysis; using regular path fallback")
			}

			if !strings.EqualFold(strings.TrimSpace(analysis.StorageEngine), "InnoDB") {
				task_output += fmt.Sprintf("Statement %d warning: storage engine is %s\n", i, analysis.StorageEngine)
			}

			if !analysis.OKOnlinePhysicalBackup {
				task_output += fmt.Sprintf("Statement %d note: online physical backup is not available\n", i)
			}

			logger.Info("* Statement ", i, " - executing schema changes on ", tableName, "...")
			_, err = executor.Execute(phase2.ExecuteOptions{
				SQL:                     statement,
				TableName:               tableName,
				BackupMethod:            phase2.BackupNone,
				UsePTOnlineSchemaChange: usePTOSC,
				Config:                  configuration,
				Debug:                   true,
			})
			if err != nil {
				logger.Error(err)
				task_output += err.Error() + "\n"
				logger.Info("* Schema changes execution failed: ", err.Error())
				task_exit_code = 1
				task_status = 4
				continue
			}

			executed++
			logger.Info("* Schema changes execution successful for statement ", i)
		}

		if executed == 0 && len(details) > 0 {
			task_exit_code = 1
			task_status = 4
			task_output = task_output + "No schema changes were executed.\n"
		}
	}
	return task_exit_code, task_status, task_output, task_error
}
