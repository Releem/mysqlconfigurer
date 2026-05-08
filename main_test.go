package main

import "testing"

func TestShouldRunOneShotMode(t *testing.T) {
	tests := []struct {
		name              string
		setConfig         bool
		getConfig         bool
		initialConfig     bool
		agentEvent        string
		agentTask         string
		serviceCommandLen int
		want              bool
	}{
		{
			name:              "runs daemon when no one-shot flags are set",
			serviceCommandLen: 0,
			want:              false,
		},
		{
			name:              "runs daemon management commands through service manager",
			setConfig:         true,
			serviceCommandLen: 1,
			want:              false,
		},
		{
			name:              "runs generate config directly",
			setConfig:         true,
			serviceCommandLen: 0,
			want:              true,
		},
		{
			name:              "runs download config directly",
			getConfig:         true,
			serviceCommandLen: 0,
			want:              true,
		},
		{
			name:              "runs initial config directly",
			initialConfig:     true,
			serviceCommandLen: 0,
			want:              true,
		},
		{
			name:              "runs event directly",
			agentEvent:        "config_applied",
			serviceCommandLen: 0,
			want:              true,
		},
		{
			name:              "runs named task directly",
			agentTask:         "queries_optimization",
			serviceCommandLen: 0,
			want:              true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldRunOneShotMode(tt.serviceCommandLen, tt.setConfig, tt.getConfig, tt.initialConfig, tt.agentEvent, tt.agentTask)
			if got != tt.want {
				t.Fatalf("shouldRunOneShotMode() = %v, want %v", got, tt.want)
			}
		})
	}
}
