package phase2

import (
	"testing"
)

func TestExtractTableNameFromDDL(t *testing.T) {
	tests := []struct {
		name    string
		ddl     string
		want    string
		wantErr bool
	}{
		{
			name:    "simple table name",
			ddl:     "ALTER TABLE users ADD COLUMN email VARCHAR(255)",
			want:    "users",
			wantErr: false,
		},
		{
			name:    "database.table format",
			ddl:     "ALTER TABLE mydb.users ADD COLUMN email VARCHAR(255)",
			want:    "mydb.users",
			wantErr: false,
		},
		{
			name:    "backticked table name",
			ddl:     "ALTER TABLE `users` ADD COLUMN email VARCHAR(255)",
			want:    "users",
			wantErr: false,
		},
		{
			name:    "backticked database.table",
			ddl:     "ALTER TABLE `mydb`.`users` ADD COLUMN email VARCHAR(255)",
			want:    "mydb.users",
			wantErr: false,
		},
		{
			name:    "not ALTER TABLE",
			ddl:     "SELECT * FROM users",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractTableNameFromDDL(tt.ddl)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractTableNameFromDDL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ExtractTableNameFromDDL() = %v, want %v", got, tt.want)
			}
		})
	}
}

