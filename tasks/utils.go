package tasks

import (
	"bytes"
	"os/exec"

	logging "github.com/google/logger"
)

// func execCmd(cmd_path string, environment []string, logger logging.Logger) (int, int, string) {
// 	return execTaskCommand(shellCommand(runtime.GOOS, cmd_path, environment), logger)
// }

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
