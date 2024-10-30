package metrics

import (
	"database/sql"
	"sort"
	"strconv"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
	"github.com/advantageous/go-logback/logging"
)

type DbMetricsBaseGatherer struct {
	logger        logging.Logger
	configuration *config.Config
}

func NewDbMetricsBaseGatherer(logger logging.Logger, configuration *config.Config) *DbMetricsBaseGatherer {

	if logger == nil {
		if configuration.Debug {
			logger = logging.NewSimpleDebugLogger("DbMetricsBase")
		} else {
			logger = logging.NewSimpleLogger("DbMetricsBase")
		}
	}

	return &DbMetricsBaseGatherer{
		logger:        logger,
		configuration: configuration,
	}
}

func (DbMetricsBase *DbMetricsBaseGatherer) GetMetrics(metrics *models.Metrics) error {
	defer utils.HandlePanic(DbMetricsBase.configuration, DbMetricsBase.logger)
	// Mysql Status
	output := make(models.MetricGroupValue)
	{
		var row models.MetricValue
		rows, err := models.DB.Query("SHOW STATUS")

		if err != nil {
			DbMetricsBase.logger.Error(err)
			return err
		}
		for rows.Next() {
			err := rows.Scan(&row.Name, &row.Value)
			if err != nil {
				DbMetricsBase.logger.Error(err)
				return err
			}
			output[row.Name] = row.Value
		}
		rows.Close()

		rows, err = models.DB.Query("SHOW GLOBAL STATUS")
		if err != nil {
			DbMetricsBase.logger.Error(err)
			return err
		}
		for rows.Next() {
			err := rows.Scan(&row.Name, &row.Value)
			if err != nil {
				DbMetricsBase.logger.Error(err)
				return err
			}
			output[row.Name] = row.Value
		}
		metrics.DB.Metrics.Status = output
		rows.Close()
	}
	// Latency
	{
		var output []models.MetricGroupValue
		var digest string
		var calls, avg_time_us int

		rows, err := models.DB.Query("SELECT CONCAT(IFNULL(schema_name, 'NULL'), '_', IFNULL(digest, 'NULL')) as queryid, count_star as calls, round(avg_timer_wait/1000000, 0) as avg_time_us FROM performance_schema.events_statements_summary_by_digest")
		if err != nil {
			if err != sql.ErrNoRows {
				DbMetricsBase.logger.Error(err)
			}
		} else {
			for rows.Next() {
				err := rows.Scan(&digest, &calls, &avg_time_us)
				if err != nil {
					DbMetricsBase.logger.Error(err)
					return err
				}
				digest := models.MetricGroupValue{"queryid": digest, "calls": calls, "avg_time_us": avg_time_us}
				output = append(output, digest)
			}
		}
		if len(output) != 0 {
			totalQueryCount := len(output)
			dictQueryCount := make(map[int]int)
			listAvgTimeDistinct := make([]int, 0)
			listAvgTime := make([]int, 0)

			for _, query := range output {
				avgTime := query["avg_time_us"].(int)
				if !contains(listAvgTimeDistinct, avgTime) {
					listAvgTimeDistinct = append(listAvgTimeDistinct, avgTime)
				}
				listAvgTime = append(listAvgTime, avgTime)
			}
			sort.Sort(sort.Reverse(sort.IntSlice(listAvgTimeDistinct)))

			for _, avgTime1 := range listAvgTime {
				for _, avgTime2 := range listAvgTimeDistinct {
					if avgTime2 >= avgTime1 {
						if _, ok := dictQueryCount[avgTime2]; !ok {
							dictQueryCount[avgTime2] = 1
						} else {
							dictQueryCount[avgTime2]++
						}
					} else {
						break
					}
				}
			}

			latency := 0
			for _, avgTime := range listAvgTimeDistinct {
				if float64(dictQueryCount[avgTime])/float64(totalQueryCount) <= 0.95 {
					break
				}
				latency = avgTime
			}

			metrics.DB.Metrics.Latency = strconv.Itoa(latency)
		} else {
			metrics.DB.Metrics.Latency = ""
		}
	}
	//status innodb engine
	{
		var engine, name, status string
		err := models.DB.QueryRow("show engine innodb status").Scan(&engine, &name, &status)
		if err != nil {
			DbMetricsBase.logger.Error(err)
		} else {
			metrics.DB.Metrics.InnoDBEngineStatus = status
		}
	}
	//list of databases
	{
		var database string
		var output []string
		rows, err := models.DB.Query("SELECT table_schema FROM INFORMATION_SCHEMA.tables group BY table_schema")
		if err != nil {
			DbMetricsBase.logger.Error(err)
			return err
		}
		for rows.Next() {
			err := rows.Scan(&database)
			if err != nil {
				DbMetricsBase.logger.Error(err)
				return err
			}
			output = append(output, database)
		}
		rows.Close()
		metrics.DB.Metrics.Databases = output
	}
	DbMetricsBase.logger.Debug("collectMetrics ", metrics.DB.Metrics)
	return nil
}

func contains(arr []int, num int) bool {
	for _, n := range arr {
		if n == num {
			return true
		}
	}
	return false
}
