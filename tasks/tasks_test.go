package tasks

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	logging "github.com/google/logger"
)

type taskRepeaterStub struct {
	taskPayload string
	statuses    []models.Task
}

func (r *taskRepeaterStub) ProcessMetrics(_ models.MetricContext, metrics models.Metrics, mode models.ModeType) (string, error) {
	if mode.Name == "Task" && mode.Type == "Get" {
		return r.taskPayload, nil
	}
	if mode.Name == "Task" && mode.Type == "Status" {
		r.statuses = append(r.statuses, metrics.ReleemAgent.Tasks)
	}
	return "", nil
}

func TestProcessTaskSkipsTaskType6WhenDisabled(t *testing.T) {
	logger := *logging.Init("tasks-test", true, false, io.Discard)
	repeater := &taskRepeaterStub{
		taskPayload: `{"task_id":42,"task_type_id":6,"task_details":"{\"statements\":[{\"schema_name\":\"app\",\"ddl_statement\":\"ALTER TABLE app.users ADD COLUMN c INT\",\"analysis_results\":{\"schema_name\":\"app\",\"table_name\":\"users\",\"syntax_valid\":true,\"storage_engine\":\"InnoDB\",\"ok_online_ddl\":true,\"ok_pt_osc\":false,\"ok_online_physical_backup\":true,\"ok_pitr\":true}}]}"}`,
	}
	cfg := &config.Config{}

	ProcessTask(repeater, nil, logger, cfg)

	if len(repeater.statuses) != 2 {
		t.Fatalf("status updates = %d, want 2", len(repeater.statuses))
	}

	finalStatus := repeater.statuses[1]
	if finalStatus.ExitCode != 0 {
		t.Fatalf("final exit code = %d, want 0", finalStatus.ExitCode)
	}
	if finalStatus.Status != 1 {
		t.Fatalf("final status = %d, want 1", finalStatus.Status)
	}
	if !strings.Contains(finalStatus.Output, "TaskID=6") {
		t.Fatalf("final output = %q, want mention of TaskID=6", finalStatus.Output)
	}
	if !strings.Contains(strings.ToLower(finalStatus.Output), "skipped") {
		t.Fatalf("final output = %q, want skip reason", finalStatus.Output)
	}
}

func TestProcessTaskRunsTaskType6WhenEnabled(t *testing.T) {
	logger := *logging.Init("tasks-test", true, false, io.Discard)
	repeater := &taskRepeaterStub{
		taskPayload: `{"task_id":42,"task_type_id":6,"task_details":"not-json"}`,
	}
	cfg := &config.Config{EnableExecDDL: true}

	ProcessTask(repeater, nil, logger, cfg)

	if len(repeater.statuses) != 2 {
		t.Fatalf("status updates = %d, want 2", len(repeater.statuses))
	}

	finalStatus := repeater.statuses[1]
	if finalStatus.ExitCode != 2 {
		t.Fatalf("final exit code = %d, want 2 from ApplySchemaChanges validation", finalStatus.ExitCode)
	}
	if finalStatus.Status != 4 {
		t.Fatalf("final status = %d, want 4", finalStatus.Status)
	}

	var rawTask map[string]any
	if err := json.Unmarshal([]byte(repeater.taskPayload), &rawTask); err != nil {
		t.Fatalf("task payload must stay valid JSON: %v", err)
	}
}
