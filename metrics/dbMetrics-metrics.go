package metrics

import (
	"database/sql"
	"strings"

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

		var total_info_length uint64
		total_info_length = 0
		information_schema_processlist_fields := []string{"ID", "USER", "HOST", "DB", "COMMAND", "TIME", "STATE", "INFO"}
		rows, err := models.DB.Query("SHOW FULL PROCESSLIST")

		if err != nil {
			DbMetricsMetricsBase.logger.Error(err)
		} else {
			defer rows.Close()

			cols, err := rows.Columns()
			if err != nil {
				DbMetricsMetricsBase.logger.Error(err)
			}
			var out []map[string]any

			for rows.Next() {
				// Готовим приёмники под каждую колонку
				values := make([]any, len(cols))
				ptrs := make([]any, len(cols))
				for i := range values {
					ptrs[i] = &values[i]
				}
				err := rows.Scan(ptrs...)
				if err != nil {
					DbMetricsMetricsBase.logger.Error(err)
					return err
				}

				row := make(map[string]any, len(cols))
				for i, col := range cols {
					col_upper := strings.ToUpper(col)
					if str_contains(information_schema_processlist_fields, col_upper) {
						v := values[i]

						// Драйвер MySQL часто отдаёт []byte для текстов и чисел.
						// Преобразуем []byte → string для удобства (если нужно).
						switch vv := v.(type) {
						case []byte:
							row[col_upper] = string(vv)
						case nil:
							row[col_upper] = "NULL"
						default:
							row[col_upper] = vv // может быть nil, time.Time, int64, float64, bool и т.д.
						}
						if col_upper == "INFO" {
							total_info_length = total_info_length + uint64(len(row[col_upper].(string)))
						}
					}
				}
				out = append(out, row)
			}
			// Convert []map[string]any to []models.MetricGroupValue
			processListConverted := make([]models.MetricGroupValue, len(out))
			for i, row := range out {
				processListConverted[i] = models.MetricGroupValue(row)
			}
			metrics.DB.Metrics.ProcessList = processListConverted

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

func str_contains(slice []string, element string) bool {
	for _, v := range slice {
		if v == element {
			return true
		}
	}
	return false
}
