package main

import (
	"flag"
	"io"
	"os"
	"os/exec"
	"testing"

	logging "github.com/google/logger"
)

func TestMainUsageAndInitializationErrorCodes(t *testing.T) {
	if mode := os.Getenv("RELEEM_TEST_MAIN_RESULT"); mode != "" {
		flag.CommandLine = flag.NewFlagSet("releem-agent", flag.ContinueOnError)
		os.Args = []string{"releem-agent", "-c", "-config", os.Getenv("RELEEM_TEST_MISSING_CONFIG")}
		if mode == "usage" {
			os.Args = []string{"releem-agent", "-c", "unsupported"}
			if got := run(); got != 2 {
				t.Errorf("run(%s) = %d, want 2", mode, got)
			}
			return
		}
		run()
		// Keep an unexpected return distinct from the expected fatal exit 1.
		os.Exit(3)
		return
	}
	for _, mode := range []string{"usage", "missing_configuration"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestMainUsageAndInitializationErrorCodes$")
			cmd.Env = append(os.Environ(), "RELEEM_TEST_MAIN_RESULT="+mode, "RELEEM_TEST_MISSING_CONFIG="+t.TempDir()+"/missing.conf")
			output, err := cmd.CombinedOutput()
			if mode == "usage" {
				if err != nil {
					t.Fatalf("usage error did not return code 2 to main: %v: %s", err, output)
				}
			} else if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
				t.Fatalf("initialization failure = %v: %s; want exit 1", err, output)
			}
		})
	}
}

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
