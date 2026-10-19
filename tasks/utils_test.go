package tasks

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	logging "github.com/google/logger"
)

type downloadCapture struct {
	mode models.ModeType
	body string
	err  error
}

func (r *downloadCapture) ProcessMetrics(_ models.MetricContext, _ models.Metrics, mode models.ModeType) (string, error) {
	r.mode = mode
	return r.body, r.err
}

func TestDownloadConfigurationFile(t *testing.T) {
	logger := *logging.Init("download", false, false, io.Discard)
	for _, mode := range []ApplyModeType{ApplyModeDynamic, ApplyModeFull} {
		for _, details := range []string{"", `{"configuration_status_id":12}`, `{"configuration_status_id":12,"extra":"preserved"}`} {
			r := &downloadCapture{body: "[mysqld]\nmax_connections=200"}
			code, status, _ := DownloadConfigurationFile(&models.Metrics{}, r, &config.Config{}, logger, mode, &models.Task{TypeID: 5, Details: details})
			if code != 0 || status != 1 || r.mode.ApplyMode != string(mode) || r.mode.Type != "Get" {
				t.Fatalf("%+v %d/%d", r.mode, code, status)
			}
			if details != "" && r.mode.ConfigurationStatusID != 12 {
				t.Fatal("selection missing")
			}
		}
	}
	for _, tc := range []struct {
		body                 string
		err                  error
		wantCode, wantStatus int
	}{
		{" ", nil, 1, 4},
		{"", errors.New("offline"), 1, 4},
		// A repeater error fails the download even when part of a body arrived.
		{"partial data", errors.New("offline"), 1, 4},
	} {
		r := &downloadCapture{body: tc.body, err: tc.err}
		code, status, _ := DownloadConfigurationFile(&models.Metrics{}, r, &config.Config{}, logger, ApplyModeFull, nil)
		if code != tc.wantCode || status != tc.wantStatus {
			t.Errorf("DownloadConfigurationFile(body=%q, err=%v) = %d/%d, want %d/%d", tc.body, tc.err, code, status, tc.wantCode, tc.wantStatus)
		}
	}
}

func TestConfigurationRequestModeValidation(t *testing.T) {
	for _, details := range []string{"", "{}", "null", `{"configuration_status_id":-1}`, `{"configuration_id":12}`, `{"configuration_status_id":"1"}`, "broken"} {
		if _, err := configurationRequestMode("Get", "full", &models.Task{TypeID: 8, Details: details}); err == nil {
			t.Fatalf("accepted %s", details)
		}
	}
	for _, kind := range []string{"Get", "GetJson"} {
		for _, mode := range []string{"dynamic", "full"} {
			got, err := configurationRequestMode(kind, mode, &models.Task{TypeID: 5, Details: `{"configuration_status_id":12}`})
			if err != nil || got.ConfigurationStatusID != 12 || got.ApplyMode != mode || got.Type != kind {
				t.Fatalf("%+v %v", got, err)
			}
		}
	}
	for _, tc := range []struct {
		details, mode string
		id            int
	}{
		{`{"configuration_status_id":1}`, "full", 1},
		{`{"configuration_status_id":12}`, "dynamic", 12},
	} {
		got, err := configurationRequestMode("Get", tc.mode, &models.Task{TypeID: 8, Details: tc.details})
		if err != nil || got.ConfigurationStatusID != tc.id || got.ApplyMode != tc.mode {
			t.Errorf("configurationRequestMode(%q, %q) = %+v, %v", tc.details, tc.mode, got, err)
		}
	}
	if _, err := configurationRequestMode("Get", "invalid", nil); err == nil {
		t.Fatal("invalid apply mode accepted")
	}
	if _, err := configurationRequestMode("Get", "full", nil); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidEnvelopeNeverStartsTask(t *testing.T) {
	logger := *logging.Init("invalid-task", false, false, io.Discard)
	for _, payload := range []string{`{}`, `null`, `[]`, `{"task_id":1}`, `{"task_id":1,"task_type_id":null}`, `{"task_id":0,"task_type_id":0}`, `{"task_id":1,"task_type_id":-1}`, `{"response_type":"configuration_application_reconciliation"}`} {
		r := &taskRepeaterStub{taskPayload: payload}
		ProcessTask(r, nil, logger, &config.Config{})
		if len(r.statuses) != 0 {
			t.Fatalf("invalid task started: %s", payload)
		}
	}
}

func TestRollBackMergesDetails(t *testing.T) {
	logger := *logging.Init("rollback", false, false, io.Discard)
	for _, details := range []string{"", `{"configuration_status_id":12,"custom":"keep"}`} {
		for _, fail := range []bool{false, true} {
			task := &models.Task{ExitCode: 7, Status: 4, Output: "apply failed\n", Details: details}
			cmd := taskCommand{name: "go", args: []string{"version"}}
			if fail {
				cmd.args = []string{"unknown-rollback-test-command"}
			}
			rollBackConfigurationTask(task, cmd, logger)
			var got struct {
				RollBack struct {
					ExitCode int    `json:"exit_code"`
					Status   int    `json:"status"`
					Output   string `json:"output"`
				} `json:"rollback"`
				Custom string `json:"custom"`
			}
			if err := json.Unmarshal([]byte(task.Details), &got); err != nil {
				t.Fatal(err)
			}
			if task.ExitCode != 7 || task.Status != 4 || !strings.HasPrefix(task.Output, "apply failed\n") {
				t.Fatal("original failure lost")
			}
			if (got.RollBack.ExitCode != 0) != fail || got.RollBack.Output == "" {
				t.Fatalf("%+v", got)
			}
			if details != "" && got.Custom != "keep" {
				t.Fatal("details overwritten")
			}
		}
	}
}

func TestUnknownTaskAndInvalidSelectionReportFailure(t *testing.T) {
	logger := *logging.Init("failed-task", false, false, io.Discard)
	for _, tc := range []struct {
		payload string
		code    int
	}{
		{`{"task_id":1,"task_type_id":9}`, 4},
		{`{"task_id":1,"task_type_id":8,"task_details":"{}"}`, 1},
		// A managed rollback without any selection must not apply the latest configuration.
		{`{"task_id":1,"task_type_id":8}`, 1},
		// Explicit type 0 is valid; the empty download stops it before applying.
		{`{"task_id":1,"task_type_id":0}`, 1},
	} {
		r := &taskRepeaterStub{taskPayload: tc.payload}
		// ProcessTask waits 10 seconds before reporting; synctest skips the wait.
		synctest.Test(t, func(*testing.T) { ProcessTask(r, nil, logger, &config.Config{}) })
		if len(r.statuses) != 2 {
			t.Fatalf("ProcessTask(%s) reported %d statuses, want 2", tc.payload, len(r.statuses))
		}
		final := r.statuses[1]
		if r.statuses[0].Status != 3 || final.Status != 4 || final.ExitCode != tc.code {
			t.Fatalf("ProcessTask(%s) statuses=%+v", tc.payload, r.statuses)
		}
	}
}

// flakyGatherer succeeds until failAfter calls, as when the database stops
// responding after a configuration task restarted it.
type flakyGatherer struct {
	calls, failAfter int
}

func (g *flakyGatherer) GetMetrics(*models.Metrics) error {
	g.calls++
	if g.calls > g.failAfter {
		return errors.New("database unavailable")
	}
	return nil
}

func TestProcessTaskReportsFinalStatusWhenMetricsFailAfterTask(t *testing.T) {
	logger := *logging.Init("final-status", false, false, io.Discard)
	r := &taskRepeaterStub{taskPayload: `{"task_id":1,"task_type_id":9}`}
	gatherer := &flakyGatherer{failAfter: 1}
	synctest.Test(t, func(*testing.T) { ProcessTask(r, []models.MetricsGatherer{gatherer}, logger, &config.Config{}) })
	if gatherer.calls != 2 {
		t.Fatalf("gatherer calls = %d, want 2", gatherer.calls)
	}
	if len(r.statuses) != 2 || r.statuses[1].ExitCode != 4 || r.statuses[1].Status != 4 {
		t.Fatalf("statuses = %+v, want started and final failure", r.statuses)
	}
}

// localApplyRepeater serves the dynamic recommendations and records requests.
type localApplyRepeater struct {
	json, file string
	jsonErr    error
	modes      []string
}

func (r *localApplyRepeater) ProcessMetrics(_ models.MetricContext, _ models.Metrics, mode models.ModeType) (string, error) {
	r.modes = append(r.modes, mode.Type)
	if mode.Type == "GetJson" {
		return r.json, r.jsonErr
	}
	return r.file, nil
}

func TestApplyDynamicConfigurationLocalFetchesRecommendationsBeforeChangingFiles(t *testing.T) {
	logger := *logging.Init("local-dynamic", false, false, io.Discard)
	metrics := &models.Metrics{}
	metrics.DB.Conf.Variables = models.MetricGroupValue{"max_connections": "200"}
	for _, tc := range []struct {
		name               string
		repeater           *localApplyRepeater
		commandExit        int
		wantExit           int
		wantModes          string
		wantCommandRunning bool
	}{
		{"request_failure", &localApplyRepeater{jsonErr: errors.New("timeout"), file: "[mysqld]"}, 0, 1, "GetJson", false},
		{"invalid_json", &localApplyRepeater{json: "not json", file: "[mysqld]"}, 0, 1, "GetJson", false},
		{"empty_file", &localApplyRepeater{json: `{"max_connections":"200"}`}, 0, 1, "GetJson,Get", false},
		{"configurer_failure", &localApplyRepeater{json: `{"max_connections":"200"}`, file: "[mysqld]"}, 3, 3, "GetJson,Get", true},
		{"no_restart_failure_preserves_output", &localApplyRepeater{json: `{"max_connections":"200"}`, file: "[mysqld]"}, 7, 7, "GetJson,Get", true},
		{"success", &localApplyRepeater{json: `{"max_connections":"200"}`, file: "[mysqld]"}, 0, 0, "GetJson,Get", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			commandRan := false
			run := func(taskCommand, logging.Logger) (int, int, string) {
				commandRan = true
				if tc.commandExit != 0 {
					return tc.commandExit, 4, "configurer failed\n"
				}
				return 0, 1, ""
			}
			var code int
			var output string
			// applyLocalRecommendations waits 10 seconds; synctest skips the wait.
			synctest.Test(t, func(*testing.T) {
				code, _, output = applyDynamicConfigurationLocal(metrics, tc.repeater, logger, &config.Config{}, &models.Task{TypeID: 4}, run)
			})
			if code != tc.wantExit || commandRan != tc.wantCommandRunning || strings.Join(tc.repeater.modes, ",") != tc.wantModes {
				t.Fatalf("exit = %d, command ran = %t, requests = %v; want %d, %t, %s", code, commandRan, tc.repeater.modes, tc.wantExit, tc.wantCommandRunning, tc.wantModes)
			}
			if tc.commandExit != 0 && !strings.Contains(output, "configurer failed") {
				t.Fatalf("applyDynamicConfigurationLocal lost configurer output: %q", output)
			}
		})
	}
}

func TestDownloadConfigurationFileRejectsNilMetrics(t *testing.T) {
	logger := logging.Init("nil-download-metrics", false, false, io.Discard)
	defer logger.Close()
	r := &downloadCapture{body: "[mysqld]"}
	code, status, output := DownloadConfigurationFile(nil, r, &config.Config{}, *logger, ApplyModeFull, nil)
	if code != 1 || status != 4 || output != "metrics unavailable" || r.mode.Type != "" {
		t.Fatalf("DownloadConfigurationFile(nil) = %d/%d %q, request = %+v", code, status, output, r.mode)
	}
}
