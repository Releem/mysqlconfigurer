package metrics

import (
	"database/sql"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
	logging "github.com/google/logger"
)

type DbMetricsMetricsBaseGatherer struct {
	logger        logging.Logger
	configuration *config.Config
}

func NewDbMetricsMetricsBaseGatherer(logger logging.Logger, configuration *config.Config) *DbMetricsMetricsBaseGatherer {
	return &DbMetricsMetricsBaseGatherer{
		logger:        logger,
		configuration: configuration,
	}
}

func (DbMetricsMetricsBase *DbMetricsMetricsBaseGatherer) GetMetrics(metrics *models.Metrics) error {
	defer utils.HandlePanic(DbMetricsMetricsBase.configuration, DbMetricsMetricsBase.logger)

	// Latency
	{
		var output []models.MetricGroupValue
		var schema_name, query_id string
		var calls, avg_time_us, sum_time_us int

		rows, err := models.DB.Query("SELECT IFNULL(schema_name, 'NULL') as schema_name, IFNULL(digest, 'NULL') as query_id, count_star as calls, round(avg_timer_wait/1000000, 0) as avg_time_us, round(SUM_TIMER_WAIT/1000000, 0) as sum_time_us FROM performance_schema.events_statements_summary_by_digest")
		if err != nil {
			if err != sql.ErrNoRows {
				DbMetricsMetricsBase.logger.Error(err)
			}
		} else {
			for rows.Next() {
				err := rows.Scan(&schema_name, &query_id, &calls, &avg_time_us, &sum_time_us)
				if err != nil {
					DbMetricsMetricsBase.logger.Error(err)
					return err
				}
				output = append(output, models.MetricGroupValue{"schema_name": schema_name, "query_id": query_id, "calls": calls, "avg_time_us": avg_time_us, "sum_time_us": sum_time_us})
			}
		}
		metrics.DB.Metrics.QueriesLatency = output

		// if len(output) != 0 {
		// 	totalQueryCount := len(output)
		// 	dictQueryCount := make(map[int]int)
		// 	listAvgTimeDistinct := make([]int, 0)
		// 	listAvgTime := make([]int, 0)

		// 	for _, query := range output {
		// 		avgTime := query["avg_time_us"].(int)
		// 		if !contains(listAvgTimeDistinct, avgTime) {
		// 			listAvgTimeDistinct = append(listAvgTimeDistinct, avgTime)
		// 		}
		// 		listAvgTime = append(listAvgTime, avgTime)
		// 	}
		// 	sort.Sort(sort.Reverse(sort.IntSlice(listAvgTimeDistinct)))

		// 	for _, avgTime1 := range listAvgTime {
		// 		for _, avgTime2 := range listAvgTimeDistinct {
		// 			if avgTime2 >= avgTime1 {
		// 				if _, ok := dictQueryCount[avgTime2]; !ok {
		// 					dictQueryCount[avgTime2] = 1
		// 				} else {
		// 					dictQueryCount[avgTime2]++
		// 				}
		// 			} else {
		// 				break
		// 			}
		// 		}
		// 	}

		// 	latency := 0
		// 	for _, avgTime := range listAvgTimeDistinct {
		// 		if float64(dictQueryCount[avgTime])/float64(totalQueryCount) <= 0.95 {
		// 			break
		// 		}
		// 		latency = avgTime
		// 	}

		// 	metrics.DB.Metrics.Latency = strconv.Itoa(latency)
		// } else {
		// 	metrics.DB.Metrics.Latency = ""
		// }
	}
	// ProcessList
	{

		type information_schema_processlist_type struct {
			ID      string
			USER    string
			HOST    string
			DB      string
			COMMAND string
			TIME    string
			STATE   string
			INFO    string
		}
		var information_schema_processlist information_schema_processlist_type
		var total_info_length uint64
		total_info_length = 0

		rows, err := models.DB.Query("SELECT IFNULL(ID, 'NULL') as ID, IFNULL(USER, 'NULL') as USER, IFNULL(HOST, 'NULL') as HOST, IFNULL(DB, 'NULL') as DB, IFNULL(COMMAND, 'NULL') as COMMAND, IFNULL(TIME, 'NULL') as TIME, IFNULL(STATE, 'NULL') as STATE, IFNULL(INFO, 'NULL') as INFO FROM information_schema.PROCESSLIST ORDER BY ID")
		if err != nil {
			DbMetricsMetricsBase.logger.Error(err)
		} else {
			for rows.Next() {
				err := rows.Scan(&information_schema_processlist.ID, &information_schema_processlist.USER, &information_schema_processlist.HOST, &information_schema_processlist.DB, &information_schema_processlist.COMMAND, &information_schema_processlist.TIME, &information_schema_processlist.STATE, &information_schema_processlist.INFO)
				if err != nil {
					DbMetricsMetricsBase.logger.Error(err)
					return err
				}
				total_info_length = total_info_length + uint64(len(information_schema_processlist.INFO))
				metrics.DB.Metrics.ProcessList = append(metrics.DB.Metrics.ProcessList, models.MetricGroupValue{"ID": information_schema_processlist.ID, "USER": information_schema_processlist.USER, "HOST": information_schema_processlist.HOST, "DB": information_schema_processlist.DB, "COMMAND": information_schema_processlist.COMMAND, "TIME": information_schema_processlist.TIME, "STATE": information_schema_processlist.STATE, "INFO": information_schema_processlist.INFO})
			}
			rows.Close()

			// Limit total INFO field size to prevent memory issues
			const maxTotalSize = 32 * 1024 * 1024 // 32MB
			const minInfoLength = 64              // Minimum INFO length to preserve
			process_list_info_limit_length := 65536

			for total_info_length > maxTotalSize && process_list_info_limit_length >= minInfoLength {
				total_info_length = 0
				for i := range metrics.DB.Metrics.ProcessList {
					infoStr := metrics.DB.Metrics.ProcessList[i]["INFO"].(string)
					if len(infoStr) > process_list_info_limit_length {
						metrics.DB.Metrics.ProcessList[i]["INFO"] = infoStr[:process_list_info_limit_length]
					}
					total_info_length += uint64(len(metrics.DB.Metrics.ProcessList[i]["INFO"].(string)))
				}
				process_list_info_limit_length = process_list_info_limit_length / 2

				// Safety check to prevent infinite loop
				if process_list_info_limit_length < minInfoLength {
					DbMetricsMetricsBase.logger.Warning("INFO truncation reached minimum length, stopping truncation")
					break
				}
			}

		}
	}

	DbMetricsMetricsBase.logger.V(5).Info("CollectMetrics DbMetricsMetricsBase ", metrics.DB.Metrics)

	return nil
}

// func contains(arr []int, num int) bool {
// 	for _, n := range arr {
// 		if n == num {
// 			return true
// 		}
// 	}
// 	return false
// }
