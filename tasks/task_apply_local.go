package tasks

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
	logging "github.com/google/logger"
)

func ApplyConfLocal(metrics *models.Metrics, repeaters models.MetricsRepeater, gatherers []models.MetricsGatherer, logger logging.Logger, configuration *config.Config) (int, int, string) {
	var task_exit_code, task_status int
	var task_output string

	result_data := models.MetricGroupValue{}
	// flush_queries := []string{"flush status", "flush statistic"}
	need_restart := false
	need_privileges := false
	need_flush := false
	error_exist := false

	recommend_var := utils.ProcessRepeaters(metrics, repeaters, configuration, logger, models.ModeType{Name: "Configurations", Type: "GetJson"})
	err := json.Unmarshal([]byte(recommend_var), &result_data)
	if err != nil {
		logger.Error(err)
	}

	for key := range result_data {
		logger.Infof("%s: %v -> %v", key, metrics.DB.Conf.Variables[key], result_data[key])

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
		// 			taskStruct.Output = taskStruct.Output + err.Error()
		// 			logger.Error(err)
		// 			// if exiterr, ok := err.(*exec.ExitError); ok {
		// 			// 	taskStruct.ExitCode = exiterr.ExitCode()
		// 			// } else {
		// 			// 	taskStruct.ExitCode = 999
		// 			// }
		// 		}
		// 		// } else {
		// 		// 	taskStruct.ExitCode = 0
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
