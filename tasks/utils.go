package tasks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/models"
	"github.com/Releem/mysqlconfigurer/utils"
	logging "github.com/google/logger"
)

// AppliedConfigurationInfo selects the configuration and mode for a task download.
type AppliedConfigurationInfo struct {
	ConfigurationStatusID int `json:"configuration_status_id"`
}

func execTaskCommand(command taskCommand, logger logging.Logger) (int, int, string) {
	var stdout, stderr bytes.Buffer
	var task_exit_code, task_status int
	var task_output string

	cmd := exec.Command(command.name, command.args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	for _, env := range command.env {
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

// DownloadConfigurationFile prepares the file before either configurer can
// back up, install or restart anything. Selection is passed directly to the
// configuration request. Request, read and write errors are returned in task output.
//
// Known technical debt: the file goes to the shared staging path
// (z_aiops_mysql.cnf) that the daemon's periodic generation also writes. A
// generation finishing between this download and the configurer's copy can
// replace a selected configuration (for example a type 8 rollback target)
// with the latest one, and the task still reports success. This is accepted
// until the Bash and PowerShell configurers are replaced by Go; until then the
// fix would be a private per-task file or a lock held only around the write
// and the copy, never around metric collection.
func DownloadConfigurationFile(metrics *models.Metrics, repeater models.MetricsRepeater, configuration *config.Config, logger logging.Logger, applyMode ApplyModeType, task *models.Task) (int, int, string) {
	mode, err := configurationRequestMode("Get", string(applyMode), task)
	if err != nil {
		return 1, 4, "Failed to parse task task_details JSON: " + err.Error()
	}
	_, err = utils.ProcessRepeaters(metrics, repeater, configuration, logger, mode)
	if err != nil {
		logger.Error("Configuration download failed: ", err)
		return 1, 4, "configuration download failed: " + err.Error()
	}
	return 0, 1, "configuration downloaded successfully"
}

type recommendationRequestError struct{ err error }

func (err *recommendationRequestError) Error() string { return err.err.Error() }
func (err *recommendationRequestError) Unwrap() error { return err.err }

// requestConfigurationRecommendations marks request errors for AWS audit output.
func requestConfigurationRecommendations(metrics *models.Metrics, repeater models.MetricsRepeater, configuration *config.Config, logger logging.Logger, mode models.ModeType) (string, error) {
	body, err := utils.ProcessRepeaters(metrics, repeater, configuration, logger, mode)
	if err != nil {
		return "", &recommendationRequestError{err}
	}
	return string(body), nil
}

// configurationRequestMode carries optional task selection directly to the request.
func configurationRequestMode(requestType string, applyMode string, task *models.Task) (models.ModeType, error) {
	mode := models.ModeType{Name: "Configurations", Type: requestType, ApplyMode: applyMode}

	if applyMode != string(ApplyModeDynamic) && applyMode != string(ApplyModeFull) {
		return mode, fmt.Errorf("invalid configuration apply mode")
	}
	if task == nil {
		return mode, nil
	}
	if task.Details == "" {
		if task.TypeID == 8 {
			return mode, fmt.Errorf("managed rollback requires configuration selection")
		}
		return mode, nil
	}
	var details AppliedConfigurationInfo
	if err := json.Unmarshal([]byte(task.Details), &details); err != nil {
		return mode, err
	}
	if details.ConfigurationStatusID < 0 {
		return mode, fmt.Errorf("invalid configuration selection")
	}
	if task.TypeID == 8 && details.ConfigurationStatusID == 0 {
		return mode, fmt.Errorf("managed rollback requires configuration selection")
	}

	mode.ConfigurationStatusID = details.ConfigurationStatusID

	return mode, nil
}

// RollBack results supplement the failed application without replacing its outcome.
func rollBackConfigurationTask(task *models.Task, command taskCommand, logger logging.Logger) {
	ExitCode, Status, Output := execTaskCommand(command, logger)
	task.Output = task.Output + Output
	payload, err := json.Marshal(map[string]interface{}{"rollback": map[string]interface{}{
		"exit_code": ExitCode,
		"status":    Status,
		"output":    Output,
	}})
	if err == nil {
		var merged string
		merged, err = utils.MergeJSONStrings(task.Details, string(payload), "")
		if err == nil {
			task.Details = merged
		}
	}
	if err != nil {
		logger.Error("Failed to merge RollBack task details: ", err)
	}
	logger.Info(" * RollBack completed with code ", ExitCode)
}
