package tasks

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"
	logging "github.com/google/logger"

	config_aws "github.com/aws/aws-sdk-go-v2/config"
)

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
	if aws.ToString(dbInstance.DBInstanceStatus) != "available" {
		logger.Error("DB Instance Status '" + aws.ToString(dbInstance.DBInstanceStatus) + "' not available(" + aws.ToString(dbInstance.DBInstanceStatus) + ")")
		task_output = task_output + "DB Instance Status '" + aws.ToString(dbInstance.DBInstanceStatus) + "' not available\n"
		task_status = 4
		task_exit_code = 1
		return task_exit_code, task_status, task_output
	} else if configuration.AwsRDSParameterGroup == "" || aws.ToString(paramGroup.DBParameterGroupName) != configuration.AwsRDSParameterGroup {
		logger.Error("Parameter group '" + configuration.AwsRDSParameterGroup + "' not found or empty in DB Instance " + configuration.AwsRDSDB + "(" + aws.ToString(paramGroup.DBParameterGroupName) + ")")
		task_output = task_output + "Parameter group '" + configuration.AwsRDSParameterGroup + "' not found or empty in DB Instance " + configuration.AwsRDSDB + "(" + aws.ToString(paramGroup.DBParameterGroupName) + ")\n"
		task_status = 4
		task_exit_code = 3
		return task_exit_code, task_status, task_output
	} else if aws.ToString(paramGroup.ParameterApplyStatus) != "in-sync" {
		logger.Error("Parameter group status '" + configuration.AwsRDSParameterGroup + "' not in-sync(" + aws.ToString(paramGroup.ParameterApplyStatus) + ")")
		task_output = task_output + "Parameter group status '" + configuration.AwsRDSParameterGroup + "' not in-sync(" + aws.ToString(paramGroup.ParameterApplyStatus) + ")\n"
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
	recommend_var := utils.ProcessRepeaters(metrics, repeaters, configuration, logger, models.ModeType{Name: "Configurations", Type: "GetJson"})
	err = json.Unmarshal([]byte(recommend_var), &result_data)
	if err != nil {
		logger.Error(err)
		task_output = task_output + err.Error()
	}

	var Parameters []types.Parameter
	var value string
	for key := range result_data {
		if result_data[key] != metrics.DB.Conf.Variables[key] {
			logger.Infof("%s: %v -> %v", key, metrics.DB.Conf.Variables[key], result_data[key])
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

		if aws.ToString(dbInstance.DBInstanceStatus) != "modifying" || aws.ToString(paramGroup.ParameterApplyStatus) != "applying" {
			break
		}
		time.Sleep(3 * time.Second)
		sum = sum + 1
	}

	if sum >= wait_seconds && aws.ToString(dbInstance.DBInstanceStatus) == "modifying" && aws.ToString(paramGroup.ParameterApplyStatus) == "applying" {
		task_exit_code = 6
		task_status = 4
	} else if aws.ToString(dbInstance.DBInstanceStatus) == "available" && aws.ToString(paramGroup.ParameterApplyStatus) == "pending-reboot" {
		task_exit_code = 10
		task_status = 4
	} else if aws.ToString(dbInstance.DBInstanceStatus) == "available" && aws.ToString(paramGroup.ParameterApplyStatus) == "in-sync" {
		logger.Info("DB Instance Status available, Parameter Group Status in-sync, No pending modifications")
	} else {
		task_exit_code = 7
		task_status = 4
	}
	time.Sleep(30 * time.Second)

	return task_exit_code, task_status, task_output
}
