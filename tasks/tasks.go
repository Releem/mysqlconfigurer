package tasks

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
	"github.com/advantageous/go-logback/logging"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/aws/aws-sdk-go/aws"

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
	task := utils.ProcessRepeaters(metrics, repeaters, configuration, logger, models.ModeType{Name: "TaskGet", Type: ""})
	if task.(models.Task).TaskTypeID == nil {
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
		if configuration.InstanceType == "aws/rds" {
			output["task_exit_code"], output["task_status"], task_output = ApplyConfAwsRds(metrics, repeaters, gatherers, logger, configuration, types.ApplyMethodImmediate)
			output["task_output"] = output["task_output"].(string) + task_output

			if output["task_exit_code"] == 10 {
				output["task_exit_code"], output["task_status"], task_output = ApplyConfAwsRds(metrics, repeaters, gatherers, logger, configuration, types.ApplyMethodPendingReboot)
				output["task_output"] = output["task_output"].(string) + task_output
			}
		} else {
			output["task_exit_code"], output["task_status"], task_output = execCmd(configuration.ReleemDir+"/mysqlconfigurer.sh -s automatic", []string{"RELEEM_RESTART_SERVICE=0"}, logger)
			output["task_output"] = output["task_output"].(string) + task_output

			if output["task_exit_code"] == 0 {
				output["task_exit_code"], output["task_status"], task_output = ApplyConfLocal(metrics, repeaters, gatherers, logger, configuration)
				output["task_output"] = output["task_output"].(string) + task_output
			}
		}
	} else if TaskTypeID == 5 {
		if configuration.InstanceType == "aws/rds" {
			output["task_exit_code"], output["task_status"], task_output = ApplyConfAwsRds(metrics, repeaters, gatherers, logger, configuration, types.ApplyMethodPendingReboot)
			output["task_output"] = output["task_output"].(string) + task_output
		} else {
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
	}
	logger.Debug(output)
	logger.Println(" * Task with id -", TaskID, "and type id -", TaskTypeID, "completed with code", output["task_exit_code"])
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

func ApplyConfLocal(metrics *models.Metrics, repeaters models.MetricsRepeater, gatherers []models.MetricsGatherer, logger logging.Logger, configuration *config.Config) (int, int, string) {
	var task_exit_code, task_status int
	var task_output string

	result_data := models.MetricGroupValue{}
	// flush_queries := []string{"flush status", "flush statistic"}
	need_restart := false
	need_privileges := false
	need_flush := false
	error_exist := false

	recommend_var := utils.ProcessRepeaters(metrics, repeaters, configuration, logger, models.ModeType{Name: "Configurations", Type: "get-json"})
	err := json.Unmarshal([]byte(recommend_var.(string)), &result_data)
	if err != nil {
		logger.Error(err)
	}

	for key := range result_data {
		logger.Println(key, result_data[key], metrics.DB.Conf.Variables[key])

		if result_data[key] != metrics.DB.Conf.Variables[key] {
			query_set_var := "set global " + key + "=" + result_data[key].(string)
			_, err := models.DB.Exec(query_set_var)
			if err != nil {
				logger.Error(err)
				task_output = task_output + err.Error()
				if strings.Contains(err.Error(), "is a read only variable") {
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
	logger.Println(need_flush, need_restart, need_privileges, error_exist)
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
		}
	}
	time.Sleep(10 * time.Second)
	metrics = utils.CollectMetrics(gatherers, logger, configuration)

	return task_exit_code, task_status, task_output
}

func ApplyConfAwsRds(metrics *models.Metrics, repeaters models.MetricsRepeater, gatherers []models.MetricsGatherer,
	logger logging.Logger, configuration *config.Config, apply_method types.ApplyMethod) (int, int, string) {

	var task_exit_code, task_status int = 0, 1
	var task_output string
	var paramGroup types.DBParameterGroupStatus
	var dbInstance types.DBInstance

	// Загрузите конфигурацию AWS по умолчанию
	cfg, err := config_aws.LoadDefaultConfig(context.TODO(), config_aws.WithRegion(configuration.AwsRegion))
	if err != nil {
		logger.Errorf("Load AWS configuration FAILED, %v", err)
		task_output = task_output + err.Error()
	} else {
		logger.Println("AWS configuration loaded SUCCESS")
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
	logger.Printf("DB Instance ID: %s, DB Instance Status: %s, Parameter Group Name: %s, Parameter Group Status: %s\n", *dbInstance.DBInstanceIdentifier, *dbInstance.DBInstanceStatus, *paramGroup.DBParameterGroupName, *paramGroup.ParameterApplyStatus)
	if aws.StringValue(dbInstance.DBInstanceStatus) != "available" {
		logger.Error("DB Instance Status '" + aws.StringValue(dbInstance.DBInstanceStatus) + "' not available(" + aws.StringValue(dbInstance.DBInstanceStatus) + ")")
		task_output = task_output + "DB Instance Status '" + aws.StringValue(dbInstance.DBInstanceStatus) + "' not available\n"
		task_status = 4
		task_exit_code = 1
		return task_exit_code, task_status, task_output
	} else if configuration.AwsRDSParameterGroup == "" || aws.StringValue(paramGroup.DBParameterGroupName) != configuration.AwsRDSParameterGroup {
		logger.Error("Parametr group '" + configuration.AwsRDSParameterGroup + "' not found or empty in DB Instance " + configuration.AwsRDSDB + "(" + aws.StringValue(paramGroup.DBParameterGroupName) + ")")
		task_output = task_output + "Parametr group '" + configuration.AwsRDSParameterGroup + "' not found or empty in DB Instance " + configuration.AwsRDSDB + "(" + aws.StringValue(paramGroup.DBParameterGroupName) + ")\n"
		task_status = 4
		task_exit_code = 3
		return task_exit_code, task_status, task_output
	} else if aws.StringValue(paramGroup.ParameterApplyStatus) != "in-sync" {
		logger.Error("Parametr group status '" + configuration.AwsRDSParameterGroup + "' not in-sync(" + aws.StringValue(paramGroup.ParameterApplyStatus) + ")")
		task_output = task_output + "Parametr group status '" + configuration.AwsRDSParameterGroup + "' not in-sync(" + aws.StringValue(paramGroup.ParameterApplyStatus) + ")\n"
		task_status = 4
		task_exit_code = 2
		return task_exit_code, task_status, task_output
	}
	DbParametrsType := make(models.MetricGroupValue)

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
				DbParametrsType[*param.ParameterName] = *param.ApplyType
			}
		}
	}
	result_data := models.MetricGroupValue{}
	recommend_var := utils.ProcessRepeaters(metrics, repeaters, configuration, logger, models.ModeType{Name: "Configurations", Type: "get-json"})
	err = json.Unmarshal([]byte(recommend_var.(string)), &result_data)
	if err != nil {
		logger.Error(err)
		task_output = task_output + err.Error()
	}

	var Parameters []types.Parameter
	var value string
	need_restart := false
	for key := range result_data {
		if result_data[key] != metrics.DB.Conf.Variables[key] {
			logger.Println(key, result_data[key], metrics.DB.Conf.Variables[key])
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
				val, ok := DbParametrsType[key]
				if ok && val != "dynamic" {
					need_restart = true
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
				logger.Println("Parameter group modified successfully")
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
			logger.Println("Parameter group modified successfully")
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
		logger.Printf("DB Instance ID: %s, DB Instance Status: %s, Parameter Group Name: %s, Parameter Group Status: %s\n", *dbInstance.DBInstanceIdentifier, *dbInstance.DBInstanceStatus, *paramGroup.DBParameterGroupName, *paramGroup.ParameterApplyStatus)

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
		task_exit_code = 11
		task_status = 4
	} else if aws.StringValue(dbInstance.DBInstanceStatus) == "available" && aws.StringValue(paramGroup.ParameterApplyStatus) == "in-sync" {
		logger.Println("DB Instance Status available, Parametr Group Status in-sync, No pending modifications")
		if need_restart {
			task_exit_code = 10
			task_status = 4
		}
	} else {
		task_exit_code = 7
		task_status = 4
	}

	metrics = utils.CollectMetrics(gatherers, logger, configuration)
	return task_exit_code, task_status, task_output
}
