package tasks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/aws/aws-sdk-go/aws"
	logging "github.com/google/logger"
	"google.golang.org/api/sqladmin/v1"
	"github.com/Releem/mysqlconfigurer/task-automator/pkg/phase1"
	"github.com/Releem/mysqlconfigurer/task-automator/pkg/phase2"

	config_aws "github.com/aws/aws-sdk-go-v2/config"
)

func ProcessTaskFunc(metrics *models.Metrics, repeaters models.MetricsRepeater, gatherers []models.MetricsGatherer, logger logging.Logger, configuration *config.Config) func() {
	return func() {
		ProcessTask(metrics, repeaters, gatherers, logger, configuration)
	}
}

func ProcessTask(metrics *models.Metrics, repeaters models.MetricsRepeater, gatherers []models.MetricsGatherer, logger logging.Logger, configuration *config.Config) {
	defer utils.HandlePanic(configuration, logger)
	output := make(models.MetricGroupValue)
	//metrics := collectMetrics(gatherers, logger)
	var task_output string
	ApplySchemaChanges(metrics, repeaters, gatherers, logger, configuration)
	task := utils.ProcessRepeaters(metrics, repeaters, configuration, logger, models.ModeType{Name: "TaskGet", Type: ""})
	if task == nil || task.(models.Task).TaskTypeID == nil {
		return
	}

	TaskTypeID := *task.(models.Task).TaskTypeID
	TaskID := *task.(models.Task).TaskID
	var stdout, stderr bytes.Buffer

	output["task_id"] = TaskID
	output["task_type_id"] = TaskTypeID
	output["task_status"] = 3
	output["task_output"] = ""

	metrics.ReleemAgent.Tasks = output
	utils.ProcessRepeaters(metrics, repeaters, configuration, logger, models.ModeType{Name: "TaskStatus", Type: ""})
	logger.Info(" * Task with id - ", TaskID, " and type id - ", TaskTypeID, " is being started...")


	switch TaskTypeID {
	case 0:
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
			logger.Info(" * Task rollbacked with code ", rollback_exit_code)
		}

	case 1:
		output["task_exit_code"], output["task_status"], task_output = execCmd(configuration.ReleemDir+"/releem-agent -f", []string{}, logger)
		output["task_output"] = output["task_output"].(string) + task_output
	case 2:
		output["task_exit_code"], output["task_status"], task_output = execCmd(configuration.ReleemDir+"/mysqlconfigurer.sh -u", []string{}, logger)
		output["task_output"] = output["task_output"].(string) + task_output
	case 3:
		output["task_exit_code"], output["task_status"], task_output = execCmd(configuration.ReleemDir+"/releem-agent --task=queries_optimization", []string{}, logger)
		output["task_output"] = output["task_output"].(string) + task_output
	case 4:
		switch configuration.InstanceType {
		case "aws/rds":
			output["task_exit_code"], output["task_status"], task_output = ApplyConfAwsRds(repeaters, gatherers, logger, configuration, types.ApplyMethodImmediate)
			output["task_output"] = output["task_output"].(string) + task_output
			if output["task_exit_code"] == 0 {
				output["task_exit_code"], output["task_status"], task_output = ApplyConfAwsRds(repeaters, gatherers, logger, configuration, types.ApplyMethodPendingReboot)
				output["task_output"] = output["task_output"].(string) + task_output
			}
		case "gcp/cloudsql":
			output["task_exit_code"], output["task_status"], task_output = ApplyConfGcpCloudSQL(repeaters, gatherers, logger, configuration, types.ApplyMethodPendingReboot)
			output["task_output"] = output["task_output"].(string) + task_output

		default:
			switch runtime.GOOS {
			case "windows":
				output["task_exit_code"] = 0
				output["task_status"] = 1
				output["task_output"] = output["task_output"].(string) + "Windows is not supported apply configuration.\n"
			default: // для Linux и других UNIX-подобных систем
				output["task_exit_code"], output["task_status"], task_output = execCmd(configuration.ReleemDir+"/mysqlconfigurer.sh -s automatic", []string{"RELEEM_RESTART_SERVICE=0"}, logger)
				output["task_output"] = output["task_output"].(string) + task_output
			}

			if output["task_exit_code"] == 0 {
				output["task_exit_code"], output["task_status"], task_output = ApplyConfLocal(metrics, repeaters, gatherers, logger, configuration)
				output["task_output"] = output["task_output"].(string) + task_output
			}
		}
		metrics = utils.CollectMetrics(gatherers, logger, configuration)
	case 5:
		switch configuration.InstanceType {
		case "aws/rds":
			output["task_exit_code"], output["task_status"], task_output = ApplyConfAwsRds(repeaters, gatherers, logger, configuration, types.ApplyMethodPendingReboot)
			output["task_output"] = output["task_output"].(string) + task_output
		case "gcp/cloudsql":
			output["task_exit_code"], output["task_status"], task_output = ApplyConfGcpCloudSQL(repeaters, gatherers, logger, configuration, types.ApplyMethodPendingReboot)
			output["task_output"] = output["task_output"].(string) + task_output

		default:
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
				logger.Info(" * Task rollbacked with code ", rollback_exit_code)
			}
		}
	}
	logger.Info(" * Task with id - ", TaskID, " and type id - ", TaskTypeID, " completed with code ", output["task_exit_code"])
	metrics.ReleemAgent.Tasks = output
	utils.ProcessRepeaters(metrics, repeaters, configuration, logger, models.ModeType{Name: "TaskStatus", Type: ""})

}

func execCmd(cmd_path string, environment []string, logger logging.Logger) (int, int, string) {
	var stdout, stderr bytes.Buffer
	var task_exit_code, task_status int
	var task_output string

	cmd := exec.Command("sh", "-c", cmd_path)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	for _, env := range environment {
		cmd.Env = append(cmd.Environ(), env)
	}
	err := cmd.Run()
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


func ApplySchemaChanges(metrics *models.Metrics, repeaters models.MetricsRepeater, gatherers []models.MetricsGatherer, logger logging.Logger, configuration *config.Config) (int, int, string) {
	var task_exit_code, task_status int
	var task_output string
	var sql_statement string
	sql_statement = "ALTER TABLE airportdb.airport ADD COLUMN email VARCHAR(255)"
	logger.Info("* Validating schema changes...")

	// Use existing models.DB connection for validation
	validator := phase1.NewValidator(models.DB)
	result, err := validator.ValidateStatements([]string{sql_statement})
	if err != nil {
		logger.Error(err)
		task_output = task_output + err.Error()
		logger.Info("* Schema changes validation failed: ", err.Error())
		task_exit_code = 1
		task_status = 4
	} else {
		logger.Info("* Schema changes validation successful: ", result.Summary())
		if len(result.ValidationErrors) > 0 {
			task_output = task_output + "Validation errors: " + strings.Join(result.ValidationErrors, "; ")
			task_exit_code = 1
			task_status = 4
		} else if len(result.ValidationWarnings) > 0 {
			task_output = task_output + "Validation warnings: " + strings.Join(result.ValidationWarnings, "; ")
		}
	}

	for i, stmt := range result.Statements {
		if stmt.StorageEngineValid == true {
			logger.Info("* Statement ", i, " - InnoDB storage engine - executing schema changes...")
				executor := phase2.NewExecutor(models.DB)
				_, err = executor.Execute(phase2.ExecuteOptions{
					SQL: sql_statement,
					TableName: "airportdb.airport",
					BackupMethod: phase2.BackupNone,
					UsePTOnlineSchemaChange: true,
					Config: configuration,
					Debug: true,
				})
				if err != nil {
					logger.Error(err)
					task_output = task_output + err.Error()
					logger.Info("* Schema changes execution failed: ", err.Error())
					task_exit_code = 1
					task_status = 4
				} else {
					logger.Info("* Schema changes execution successful: ", result.Summary())
				}
		}
	}

	return task_exit_code, task_status, task_output
}

func ApplyConfLocal(metrics *models.Metrics, repeaters models.MetricsRepeater, gatherers []models.MetricsGatherer, logger logging.Logger, configuration *config.Config) (int, int, string) {
	var task_exit_code, task_status int
	var task_output string

	result_data := models.MetricGroupValue{}
	// flush_queries := []string{"flush status", "flush statistic"}
	need_restart := false
	need_privileges := false
	need_flush := false
	error_exist := false

	recommend_var := utils.ProcessRepeaters(metrics, repeaters, configuration, logger, models.ModeType{Name: "Configurations", Type: "ForceGetJson"})
	err := json.Unmarshal([]byte(recommend_var.(string)), &result_data)
	if err != nil {
		logger.Error(err)
	}

	for key := range result_data {
		logger.Info(key, result_data[key], metrics.DB.Conf.Variables[key])

		if result_data[key] != metrics.DB.Conf.Variables[key] {
			query_set_var := "set global " + key + "=" + result_data[key].(string)
			_, err := models.DB.Exec(query_set_var)
			if err != nil {
				logger.Error(err)
				task_output = task_output + err.Error()
				if strings.Contains(err.Error(), "is a read only variable") || strings.Contains(err.Error(), "innodb_log_file_size must be at least") {
					need_restart = true
				} else if strings.Contains(err.Error(), "Access denied") {
					need_privileges = true
				} else {
					error_exist = true
				}
			} else {
				need_flush = true
			}
		}
	}
	logger.Info(need_flush, need_restart, need_privileges, error_exist)
	if error_exist {
		task_exit_code = 8
		task_status = 4
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
			task_exit_code = 9
			task_status = 4
		} else if need_restart {
			task_exit_code = 10
			task_status = 1
		} else {
			task_exit_code = 0
			task_status = 1
		}
	}
	time.Sleep(10 * time.Second)

	return task_exit_code, task_status, task_output
}

func ApplyConfAwsRds(repeaters models.MetricsRepeater, gatherers []models.MetricsGatherer,
	logger logging.Logger, configuration *config.Config, apply_method types.ApplyMethod) (int, int, string) {

	var task_exit_code, task_status int = 0, 1
	var task_output string
	var paramGroup types.DBParameterGroupStatus
	var dbInstance types.DBInstance

	metrics := utils.CollectMetrics(gatherers, logger, configuration)

	// Загрузите конфигурацию AWS по умолчанию
	cfg, err := config_aws.LoadDefaultConfig(context.TODO(), config_aws.WithRegion(configuration.AwsRegion))
	if err != nil {
		logger.Errorf("Load AWS configuration FAILED, %v", err)
		task_output = task_output + err.Error()
	} else {
		logger.Info("AWS configuration loaded SUCCESS")
	}

	// Создайте клиент RDS
	rdsclient := rds.NewFromConfig(cfg)

	// Prepare request to RDS
	input := &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: &configuration.AwsRDSDB,
	}
	result, err := rdsclient.DescribeDBInstances(context.TODO(), input)
	if err != nil {
		logger.Errorf("Failed to describe DB instance: %v", err)
		task_output = task_output + err.Error()
	}

	// Проверяем статус инстанса и требуются ли изменения
	if len(result.DBInstances) > 0 {
		dbInstance = result.DBInstances[0]
		paramGroup = dbInstance.DBParameterGroups[0]
	} else {
		logger.Error("No DB instance found.")
		task_output = task_output + "No DB instance found.\n"
	}
	logger.Infof("DB Instance ID: %s, DB Instance Status: %s, Parameter Group Name: %s, Parameter Group Status: %s\n", *dbInstance.DBInstanceIdentifier, *dbInstance.DBInstanceStatus, *paramGroup.DBParameterGroupName, *paramGroup.ParameterApplyStatus)
	if aws.StringValue(dbInstance.DBInstanceStatus) != "available" {
		logger.Error("DB Instance Status '" + aws.StringValue(dbInstance.DBInstanceStatus) + "' not available(" + aws.StringValue(dbInstance.DBInstanceStatus) + ")")
		task_output = task_output + "DB Instance Status '" + aws.StringValue(dbInstance.DBInstanceStatus) + "' not available\n"
		task_status = 4
		task_exit_code = 1
		return task_exit_code, task_status, task_output
	} else if configuration.AwsRDSParameterGroup == "" || aws.StringValue(paramGroup.DBParameterGroupName) != configuration.AwsRDSParameterGroup {
		logger.Error("Parameter group '" + configuration.AwsRDSParameterGroup + "' not found or empty in DB Instance " + configuration.AwsRDSDB + "(" + aws.StringValue(paramGroup.DBParameterGroupName) + ")")
		task_output = task_output + "Parameter group '" + configuration.AwsRDSParameterGroup + "' not found or empty in DB Instance " + configuration.AwsRDSDB + "(" + aws.StringValue(paramGroup.DBParameterGroupName) + ")\n"
		task_status = 4
		task_exit_code = 3
		return task_exit_code, task_status, task_output
	} else if aws.StringValue(paramGroup.ParameterApplyStatus) != "in-sync" {
		logger.Error("Parameter group status '" + configuration.AwsRDSParameterGroup + "' not in-sync(" + aws.StringValue(paramGroup.ParameterApplyStatus) + ")")
		task_output = task_output + "Parameter group status '" + configuration.AwsRDSParameterGroup + "' not in-sync(" + aws.StringValue(paramGroup.ParameterApplyStatus) + ")\n"
		task_status = 4
		task_exit_code = 2
		return task_exit_code, task_status, task_output
	}
	DbParametersType := make(models.MetricGroupValue)

	if apply_method == types.ApplyMethodImmediate {
		// Вызов DescribeDBParameters для получения параметров группы
		input := &rds.DescribeDBParametersInput{
			DBParameterGroupName: aws.String(configuration.AwsRDSParameterGroup),
		}

		// Итерируем по всем параметрам в группе и выводим ApplyType для каждого
		paginator := rds.NewDescribeDBParametersPaginator(rdsclient, input)
		for paginator.HasMorePages() {
			page, err := paginator.NextPage(context.TODO())
			if err != nil {
				logger.Errorf("Failed to retrieve parameters: %v", err)
				task_output = task_output + err.Error()
			}
			for _, param := range page.Parameters {
				DbParametersType[*param.ParameterName] = *param.ApplyType
			}
		}
	}
	result_data := models.MetricGroupValue{}
	recommend_var := utils.ProcessRepeaters(metrics, repeaters, configuration, logger, models.ModeType{Name: "Configurations", Type: "ForceGetJson"})
	err = json.Unmarshal([]byte(recommend_var.(string)), &result_data)
	if err != nil {
		logger.Error(err)
		task_output = task_output + err.Error()
	}

	var Parameters []types.Parameter
	var value string
	for key := range result_data {
		if result_data[key] != metrics.DB.Conf.Variables[key] {
			logger.Info(key, result_data[key], metrics.DB.Conf.Variables[key])
			if key == "innodb_max_dirty_pages_pct" {
				i, err := strconv.ParseFloat(result_data[key].(string), 32)
				if err != nil {
					logger.Error(err)
					task_output = task_output + err.Error()
				}
				value = strconv.Itoa(int(i))
			} else {
				value = result_data[key].(string)
			}

			if apply_method == types.ApplyMethodImmediate {
				val, ok := DbParametersType[key]
				if ok && val != "dynamic" {
					continue
				}
			}
			Parameters = append(Parameters, types.Parameter{
				ParameterName:  aws.String(key),
				ParameterValue: aws.String(value),
				ApplyMethod:    apply_method,
			})
		}
		if len(Parameters) == 20 {
			// Создайте запрос на изменение параметра в группе параметров
			input := &rds.ModifyDBParameterGroupInput{
				DBParameterGroupName: aws.String(configuration.AwsRDSParameterGroup),
				Parameters:           Parameters,
			}

			// Вызовите ModifyDBParameterGroup API для изменения параметра
			_, err := rdsclient.ModifyDBParameterGroup(context.TODO(), input)
			if err != nil {
				if strings.Contains(err.Error(), "AccessDenied") {
					task_exit_code = 9
					task_status = 4
				} else {
					task_exit_code = 8
					task_status = 4
				}
				logger.Errorf("Parameter group modified unsuccessfully: %v", err)
				task_output = task_output + err.Error()
				return task_exit_code, task_status, task_output
			} else {
				logger.Info("Parameter group modified successfully")
			}
			Parameters = []types.Parameter{}
		}
	}
	if len(Parameters) != 0 {
		// Создайте запрос на изменение параметра в группе параметров
		input := &rds.ModifyDBParameterGroupInput{
			DBParameterGroupName: aws.String(configuration.AwsRDSParameterGroup),
			Parameters:           Parameters,
		}

		// Вызовите ModifyDBParameterGroup API для изменения параметра
		_, err := rdsclient.ModifyDBParameterGroup(context.TODO(), input)
		if err != nil {
			if strings.Contains(err.Error(), "AccessDenied") {
				task_exit_code = 9
				task_status = 4
			} else {
				task_exit_code = 8
				task_status = 4
			}
			logger.Errorf("Parameter group modified unsuccessfully: %v", err)
			task_output = task_output + err.Error()
			return task_exit_code, task_status, task_output
		} else {
			logger.Info("Parameter group modified successfully")
		}
	}
	time.Sleep(15 * time.Second)
	sum := 1
	wait_seconds := 400
	for sum < wait_seconds {
		// Prepare request to RDS
		input = &rds.DescribeDBInstancesInput{
			DBInstanceIdentifier: &configuration.AwsRDSDB,
		}
		result, err = rdsclient.DescribeDBInstances(context.TODO(), input)
		if err != nil {
			logger.Errorf("Failed to describe DB instance: %v", err)
			task_output = task_output + err.Error()
		}
		// Проверяем статус инстанса и требуются ли изменения
		if len(result.DBInstances) > 0 {
			dbInstance = result.DBInstances[0]
			paramGroup = dbInstance.DBParameterGroups[0]
		} else {
			logger.Error("No DB instance found.")
			task_output = task_output + "No DB instance found.\n"
		}
		logger.Infof("DB Instance ID: %s, DB Instance Status: %s, Parameter Group Name: %s, Parameter Group Status: %s\n", *dbInstance.DBInstanceIdentifier, *dbInstance.DBInstanceStatus, *paramGroup.DBParameterGroupName, *paramGroup.ParameterApplyStatus)

		if aws.StringValue(dbInstance.DBInstanceStatus) != "modifying" || aws.StringValue(paramGroup.ParameterApplyStatus) != "applying" {
			break
		}
		time.Sleep(3 * time.Second)
		sum = sum + 1
	}

	if sum >= wait_seconds && aws.StringValue(dbInstance.DBInstanceStatus) == "modifying" && aws.StringValue(paramGroup.ParameterApplyStatus) == "applying" {
		task_exit_code = 6
		task_status = 4
	} else if aws.StringValue(dbInstance.DBInstanceStatus) == "available" && aws.StringValue(paramGroup.ParameterApplyStatus) == "pending-reboot" {
		task_exit_code = 10
		task_status = 4
	} else if aws.StringValue(dbInstance.DBInstanceStatus) == "available" && aws.StringValue(paramGroup.ParameterApplyStatus) == "in-sync" {
		logger.Info("DB Instance Status available, Parameter Group Status in-sync, No pending modifications")
	} else {
		task_exit_code = 7
		task_status = 4
	}
	time.Sleep(30 * time.Second)

	return task_exit_code, task_status, task_output
}

func ApplyConfGcpCloudSQL(repeaters models.MetricsRepeater, gatherers []models.MetricsGatherer,
	logger logging.Logger, configuration *config.Config, apply_method types.ApplyMethod) (int, int, string) {

	var task_exit_code, task_status int = 0, 1
	var task_output string

	metrics := utils.CollectMetrics(gatherers, logger, configuration)

	// Initialize GCP clients with Application Default Credentials
	ctx := context.Background()

	// Create SQL Admin client
	sqlAdminService, err := sqladmin.NewService(ctx)
	if err != nil {
		logger.Error("Failed to create GCP SQL Admin client", err)
		task_output = task_output + "Failed to create GCP SQL Admin client" + err.Error()
		task_status = 4
		task_exit_code = 1
		return task_exit_code, task_status, task_output
	}
	logger.Info("GSP configuration loaded SUCCESS")
	task_output = task_output + "GSP configuration loaded SUCCESS\n"
	// Get instance details
	instance, err := sqlAdminService.Instances.Get(configuration.GcpProjectId, configuration.GcpCloudSqlInstance).Do()
	if err != nil {
		logger.Error("Failed to get Cloud SQL instance details", err)
		task_output = task_output + "Failed to get Cloud SQL instance details" + err.Error()
		task_status = 4
		task_exit_code = 1
		return task_exit_code, task_status, task_output
	}

	logger.Info("GCP Cloud SQL instance found: ", instance.Name)
	task_output = task_output + "GCP Cloud SQL instance found: " + instance.Name + "\n"

	// Get current database flags from the instance
	currentFlags := make(map[string]string)
	if instance.Settings != nil && instance.Settings.DatabaseFlags != nil {
		for _, flag := range instance.Settings.DatabaseFlags {
			currentFlags[flag.Name] = flag.Value
		}
		logger.Infof("Current database flags count: %d", len(currentFlags))
	}

	recommendedVars := models.MetricGroupValue{}
	recommend_var := utils.ProcessRepeaters(metrics, repeaters, configuration, logger, models.ModeType{Name: "Configurations", Type: "ForceGetJson"})
	err = json.Unmarshal([]byte(recommend_var.(string)), &recommendedVars)
	if err != nil {
		logger.Error(err)
		task_output = task_output + err.Error()
	}

	// Merge current flags with recommended changes
	// mergeGcpDatabaseFlags merges current database flags with recommended changes
	// preserving existing flags that are not being modified
	var mergedFlags []*sqladmin.DatabaseFlags
	processedFlags := make(map[string]bool)

	// First, add all recommended changes
	for key := range recommendedVars {
		if recommendedVars[key] != metrics.DB.Conf.Variables[key] {
			logger.Infof("Updating flag %s: current=%v, recommended=%v, db_current=%v",
				key, currentFlags[key], recommendedVars[key], metrics.DB.Conf.Variables[key])
			task_output = task_output + fmt.Sprintf("Updating flag %s: current=%v, recommended=%v, db_current=%v\n",
				key, currentFlags[key], recommendedVars[key], metrics.DB.Conf.Variables[key])
			value := recommendedVars[key].(string)
			mergedFlags = append(mergedFlags, &sqladmin.DatabaseFlags{
				Name:  key,
				Value: value,
			})
			processedFlags[key] = true
		}
	}

	// Then, preserve all existing flags that weren't modified
	for flagName, flagValue := range currentFlags {
		if !processedFlags[flagName] {
			logger.Infof("Preserving existing flag %s=%s", flagName, flagValue)
			task_output = task_output + fmt.Sprintf("Preserving existing flag %s=%s\n", flagName, flagValue)
			mergedFlags = append(mergedFlags, &sqladmin.DatabaseFlags{
				Name:  flagName,
				Value: flagValue,
			})
		}
	}

	logger.Infof("Total flags to apply: %d (recommended changes: %d, preserved: %d)",
		len(mergedFlags), len(processedFlags), len(mergedFlags)-len(processedFlags))
	task_output = task_output + fmt.Sprintf("Total flags to apply: %d (recommended changes: %d, preserved: %d)\n",
		len(mergedFlags), len(processedFlags), len(mergedFlags)-len(processedFlags))

	req := &sqladmin.DatabaseInstance{
		Settings: &sqladmin.Settings{
			DatabaseFlags: mergedFlags,
		},
	}
	// A partial update (Patch) is safer than a full update (Update) because we only change the specified fields.
	op, err := sqlAdminService.Instances.Patch(configuration.GcpProjectId, configuration.GcpCloudSqlInstance, req).Context(ctx).Do()
	if err != nil {
		if strings.Contains(err.Error(), "notAuthorized") {
			task_exit_code = 9
			task_status = 4
		} else {
			task_exit_code = 8
			task_status = 4
		}
		logger.Errorf("Instances.Patch: %v", err)
		task_output = task_output + err.Error()
		return task_exit_code, task_status, task_output
	} else {
		logger.Info("Instances.Patch modified successfully")
		task_output = task_output + "Instances.Patch modified successfully.\n"
	}

	if err := waitForOp(ctx, sqlAdminService, configuration.GcpProjectId, op); err != nil {
		logger.Fatalf("waitForOp: %v", err)
		task_exit_code = 8
		task_status = 4
		task_output = task_output + err.Error()
	} else {
		task_output = task_output + "Cloud SQL instance updated successfully.\n"
		logger.Info("Cloud SQL instance updated successfully.")
	}

	return task_exit_code, task_status, task_output

}

// Waiting for long Cloud SQL operations to complete
func waitForOp(ctx context.Context, sqlAdminService *sqladmin.Service, project string, op *sqladmin.Operation) error {
	opName := op.Name
	for {
		cur, err := sqlAdminService.Operations.Get(project, opName).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("operations.get: %w", err)
		}
		if cur.Status == "DONE" {
			if cur.Error != nil && len(cur.Error.Errors) > 0 {
				return fmt.Errorf("cloudsql op error: %v", cur.Error.Errors[0].Message)
			}
			return nil
		}
		time.Sleep(3 * time.Second)
	}
}
