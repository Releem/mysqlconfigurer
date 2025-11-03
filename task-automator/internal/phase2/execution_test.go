package phase2

import (
	"testing"
)

func TestBackupMethod(t *testing.T) {
	tests := []struct {
		name string
		bm   BackupMethod
		want string
	}{
		{
			name: "none",
			bm:   BackupNone,
			want: "none",
		},
		{
			name: "mysqldump",
			bm:   BackupMysqldump,
			want: "mysqldump",
		},
		{
			name: "xtrabackup",
			bm:   BackupXtrabackup,
			want: "xtrabackup",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.bm) != tt.want {
				t.Errorf("BackupMethod = %v, want %v", tt.bm, tt.want)
			}
		})
	}
}


