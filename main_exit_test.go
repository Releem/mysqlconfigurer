package main

import (
	"io"
	"os"
	"os/exec"
	"testing"

	logging "github.com/google/logger"
)

func TestRunErrorExitCodes(t *testing.T) {
	if mode := os.Getenv("RELEEM_TEST_EXIT_MODE"); mode != "" {
		logger = *logging.Init("exit-code-test", false, false, io.Discard)
		if mode == "usage" {
			exitRunWithCode(2, "invalid apply mode")
		} else {
			exitRunWithError("configuration unavailable")
		}
		return
	}
	for _, tc := range []struct {
		mode string
		code int
	}{{"default", 1}, {"usage", 2}} {
		t.Run(tc.mode, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestRunErrorExitCodes$")
			cmd.Env = append(os.Environ(), "RELEEM_TEST_EXIT_MODE="+tc.mode)
			err := cmd.Run()
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != tc.code {
				t.Fatalf("error exit mode %s: got %v, want exit code %d", tc.mode, err, tc.code)
			}
		})
	}
}
