package metrics

import (
	"database/sql"
	"sort"
	"strconv"

	"github.com/Releem/mysqlconfigurer/config"
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

func (DbMetricsBase *DbMetricsBaseGatherer) GetMetrics(metrics *Metrics) error {
	defer HandlePanic(DbMetricsBase.configuration, DbMetricsBase.logger)
	// Mysql Status
	output := make(MetricGroupValue)
	{
		var row MetricValue
		rows, err := config.DB.Query("SHOW STATUS")

		if err != nil {
			DbMetricsBase.logger.Error(err)
			return err
		}
		for rows.Next() {
			err := rows.Scan(&row.name, &row.value)
			if err != nil {
				DbMetricsBase.logger.Error(err)
				return err
			}
			output[row.name] = row.value
		}
		rows.Close()

		rows, err = config.DB.Query("SHOW GLOBAL STATUS")
		if err != nil {
			DbMetricsBase.logger.Error(err)
			return err
		}
		for rows.Next() {
			err := rows.Scan(&row.name, &row.value)
			if err != nil {
				DbMetricsBase.logger.Error(err)
				return err
			}
			output[row.name] = row.value
		}
		metrics.DB.Metrics.Status = output
		rows.Close()
	}
	// Latency
	{
		var output []MetricGroupValue
		var digest string
		var calls, avg_time_us int

		rows, err := config.DB.Query("SELECT CONCAT(IFNULL(schema_name, 'NULL'), '_', IFNULL(digest, 'NULL')) as queryid, count_star as calls, round(avg_timer_wait/1000000, 0) as avg_time_us FROM performance_schema.events_statements_summary_by_digest")
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
				digest := MetricGroupValue{"queryid": digest, "calls": calls, "avg_time_us": avg_time_us}
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
		err := config.DB.QueryRow("show engine innodb status").Scan(&engine, &name, &status)
		if err != nil {
			DbMetricsBase.logger.Error(err)
		} else {
			metrics.DB.Metrics.InnoDBEngineStatus = status
		}
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
