package tasks

import (
	"reflect"
	"testing"
)

func TestWindowsTaskCommands(t *testing.T) {
	releemDir := `C:\Program Files\ReleemAgent`

	tests := []struct {
		name string
		cmd  taskCommand
		want taskCommand
	}{
		{
			name: "type 0 applies config non-interactively",
			cmd:  taskApplyManualCommand("windows", releemDir),
			want: taskCommand{
				name: "powershell.exe",
				args: []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", `C:\Program Files\ReleemAgent\releem.ps1`, "-Apply", "-NonInteractive"},
			},
		},
		{
			name: "type 1 generates config",
			cmd:  taskGenerateConfigCommand("windows", releemDir),
			want: taskCommand{
				name: `C:\Program Files\ReleemAgent\releem-agent.exe`,
				args: []string{"-f"},
			},
		},
		{
			name: "type 3 queues query optimization",
			cmd:  taskQueriesOptimizationCommand("windows", releemDir),
			want: taskCommand{
				name: `C:\Program Files\ReleemAgent\releem-agent.exe`,
				args: []string{"--task=queries_optimization"},
			},
		},
		{
			name: "type 4 applies config without restart",
			cmd:  taskApplyAutomaticCommand("windows", releemDir, false),
			want: taskCommand{
				name: "powershell.exe",
				args: []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", `C:\Program Files\ReleemAgent\releem.ps1`, "-Apply", "-NonInteractive", "-NoRestart"},
			},
		},
		{
			name: "type 5 applies config with restart",
			cmd:  taskApplyAutomaticCommand("windows", releemDir, true),
			want: taskCommand{
				name: "powershell.exe",
				args: []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", `C:\Program Files\ReleemAgent\releem.ps1`, "-Apply", "-NonInteractive", "-ForceRestart"},
			},
		},
		{
			name: "rollback confirms non-interactively",
			cmd:  taskRollBackCommand("windows", releemDir),
			want: taskCommand{
				name: "powershell.exe",
				args: []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", `C:\Program Files\ReleemAgent\releem.ps1`, "-Rollback"},
				env:  []string{"RELEEM_ROLLBACK_CONFIRM=1"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !reflect.DeepEqual(tt.cmd, tt.want) {
				t.Fatalf("command mismatch\nwant: %#v\n got: %#v", tt.want, tt.cmd)
			}
		})
	}
}

func TestLinuxTaskCommandsUseReleemConfigurer(t *testing.T) {
	releemDir := "/opt/releem"

	tests := []struct {
		name string
		cmd  taskCommand
		want taskCommand
	}{
		{
			name: "update uses releem.sh -u",
			cmd:  taskUpdateCommand("linux", releemDir),
			want: taskCommand{name: "sh", args: []string{"-c", "/opt/releem/releem.sh -u"}},
		},
		{
			name: "rollback uses releem.sh -r",
			cmd:  taskRollBackCommand("linux", releemDir),
			want: taskCommand{name: "sh", args: []string{"-c", "/opt/releem/releem.sh -r"}, env: []string{"RELEEM_RESTART_SERVICE=1"}},
		},
		{
			name: "type 0 uses releem.sh -a",
			cmd:  taskApplyManualCommand("linux", releemDir),
			want: taskCommand{
				name: "sh",
				args: []string{"-c", "/opt/releem/releem.sh -a"},
				env:  []string{"RELEEM_RESTART_SERVICE=1"},
			},
		},
		{
			name: "type 1 uses releem-agent -f",
			cmd:  taskGenerateConfigCommand("linux", releemDir),
			want: taskCommand{
				name: "sh",
				args: []string{"-c", "/opt/releem/releem-agent -f"},
			},
		},
		{
			name: "type 3 uses releem-agent query optimization task",
			cmd:  taskQueriesOptimizationCommand("linux", releemDir),
			want: taskCommand{
				name: "sh",
				args: []string{"-c", "/opt/releem/releem-agent --task=queries_optimization"},
			},
		},
		{
			name: "type 4 uses automatic apply without restart",
			cmd:  taskApplyAutomaticCommand("linux", releemDir, false),
			want: taskCommand{
				name: "sh",
				args: []string{"-c", "/opt/releem/releem.sh -s automatic"},
				env:  []string{"RELEEM_RESTART_SERVICE=0"},
			},
		},
		{
			name: "type 5 uses automatic apply with restart",
			cmd:  taskApplyAutomaticCommand("linux", releemDir, true),
			want: taskCommand{
				name: "sh",
				args: []string{"-c", "/opt/releem/releem.sh -s automatic"},
				env:  []string{"RELEEM_RESTART_SERVICE=1"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !reflect.DeepEqual(tt.cmd, tt.want) {
				t.Fatalf("command mismatch\nwant: %#v\n got: %#v", tt.want, tt.cmd)
			}
		})
	}
}

func TestUpdateCommandRemainsAvailable(t *testing.T) {
	if got := taskUpdateCommand("linux", "/agent"); !reflect.DeepEqual(got.args, []string{"-c", "/agent/releem.sh -u"}) {
		t.Fatal(got)
	}
	got := taskUpdateCommand("windows", `C:\Agent`)
	if got.args[len(got.args)-1] != "-Update" {
		t.Fatal(got)
	}
}
