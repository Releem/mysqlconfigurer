package task

import (
	"encoding/json"
	"fmt"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/metrics/mysql"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"

	logging "github.com/google/logger"
)

type QueryExplainTaskInput struct {
	SchemaName string `json:"schema_name"`
	QueryText  string `json:"query_text"`
}

type QueryExplainTaskResult struct {
	Schema                  map[string][]models.MetricGroupValue `json:"schema"`
	Explain                 string                               `json:"explain"`
	EventsStatementsHistory models.MetricGroupValue              `json:"events_statements_history"`
}

func ProcessQueryExplainTask(task_details string, logger logging.Logger, configuration *config.Config, metrics *models.Metrics) (int, int, string) {
	var task_exit_code, task_status int = 0, 1
	var task_output string

	logger.Info(task_details)
	// Parse JSON from Task Details
	var inputs []QueryExplainTaskInput
	err := json.Unmarshal([]byte(task_details), &inputs)
	if err != nil {
		logger.Error("Failed to parse task task_details JSON: ", err)
		task_exit_code = 8
		task_status = 4
		task_output = fmt.Sprintf("Error parsing JSON: %v\n", err)
		return task_exit_code, task_status, task_output
	}

	if len(inputs) == 0 {
		logger.Error("task_details array is empty")
		task_exit_code = 8
		task_status = 4
		task_output = "Error: task_details must contain at least one item\n"
		return task_exit_code, task_status, task_output
	}

	metrics.DB.DatabaseSchema = make(map[string][]models.MetricGroupValue)
	metrics.DB.Queries = make([]models.MetricGroupValue, 0, len(inputs))
	collectedSchemas := make(map[string]struct{})

	for _, input := range inputs {
		if input.SchemaName == "" || input.QueryText == "" {
			logger.Error("schema_name or query_text is empty")
			task_exit_code = 8
			task_status = 4
			task_output = "Error: schema_name and query_text are required\n"
			return task_exit_code, task_status, task_output
		}

		if _, ok := collectedSchemas[input.SchemaName]; !ok {
			// Collect schema
			err = mysql.CollectDbSchema(input.SchemaName, logger, metrics)
			if err != nil {
				logger.Error("Failed to collect schema: ", err)
				task_exit_code = 8
				task_status = 4
				task_output = fmt.Sprintf("Error collecting schema: %v\n", err)
				return task_exit_code, task_status, task_output
			}
			collectedSchemas[input.SchemaName] = struct{}{}
		}

		// Connect to the database
		db := utils.ConnectionDatabase(configuration, logger, input.SchemaName)

		// // Get THREAD_ID before executing EXPLAIN (using the same connection)
		// var threadID uint64
		// err = db.QueryRow("SELECT THREAD_ID FROM performance_schema.threads WHERE PROCESSLIST_ID = CONNECTION_ID()").Scan(&threadID)
		// if err != nil {
		// 	logger.Error("Failed to get THREAD_ID: ", err)
		// 	task_exit_code = 8
		// 	task_status = 4
		// 	task_output = fmt.Sprintf("Error getting THREAD_ID: %v\n", err)
		// 	return task_exit_code, task_status, task_output
		// }
		// logger.Info("THREAD_ID: ", threadID)

		// Execute EXPLAIN
		explainResult, explain_error := mysql.ExecuteExplain(db, input.QueryText, logger)
		// explainResult, err := executeExplain(db, input.QueryText, logger)
		if explain_error != nil {
			logger.Error("Failed to execute EXPLAIN: ", explain_error)
			// Continue even if EXPLAIN fails, but note it in the result
			explainResult = fmt.Sprintf("Error: %v", explain_error)
		}

		// // Get last row from events_statements_history for THREAD_ID
		// var Digest string
		// err = db.QueryRow("SELECT IFNULL(DIGEST, 'NULL') as digest FROM performance_schema.events_statements_history WHERE THREAD_ID = ? AND EVENT_NAME = 'statement/sql/select' ORDER BY EVENT_ID DESC LIMIT 1", threadID).Scan(&Digest)
		// if err != nil {
		// 	logger.Error("Failed to get digest hash: ", err)
		// 	task_exit_code = 8
		// 	task_status = 4
		// 	task_output = fmt.Sprintf("Failed to get digest hash: %v\n", err)
		// 	return task_exit_code, task_status, task_output
		// }
		// defer rows.Close()

		// for rows.Next() {
		// 	err := rows.Scan(&Digest, &DigestText)
		// 	if err != nil {
		// 		logger.Error("Failed to scan rows: ", err)
		// 	}
		// 	logger.Info("Digest: ", Digest, " DigestText: ", DigestText)
		// }

		db.Close()

		// Build result
		metrics.DB.Queries = append(metrics.DB.Queries, models.MetricGroupValue{
			"schema_name": input.SchemaName,
			"query_text":  input.QueryText,
			"explain":     explainResult,
			"avg_time_us": 0,
			"calls":       0,
			"sum_time_us": 0,
		})
	}

	return task_exit_code, task_status, task_output
}

// func getEventsStatementsHistory(db *sql.DB, threadID uint64, logger logging.Logger) (models.MetricGroupValue, error) {
// 	// Use SELECT * to get all columns dynamically
// 	var Digest string
// 	err := db.QueryRow("SELECT DIGEST FROM performance_schema.events_statements_history WHERE THREAD_ID = ? ORDER BY EVENT_ID DESC LIMIT 1", threadID).Scan(&Digest)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer rows.Close()

// 	if !rows.Next() {
// 		return models.MetricGroupValue{}, nil
// 	}

// 	// Get column names
// 	cols, err := rows.Columns()
// 	if err != nil {
// 		return nil, err
// 	}

// 	// Prepare values
// 	values := make([]interface{}, len(cols))
// 	ptrs := make([]interface{}, len(cols))
// 	for i := range values {
// 		ptrs[i] = &values[i]
// 	}

// 	err = rows.Scan(ptrs...)
// 	if err != nil {
// 		return nil, err
// 	}

// 	// Build result map
// 	result := make(models.MetricGroupValue)
// 	for i, col := range cols {
// 		v := values[i]
// 		switch vv := v.(type) {
// 		case []byte:
// 			result[col] = string(vv)
// 		case nil:
// 			result[col] = nil
// 		default:
// 			result[col] = vv
// 		}
// 	}

// 	return result, nil
// }
