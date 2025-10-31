package phase1

import (
	"database/sql"
	"testing"
)

// Mock connection for testing
func createMockConnection() *sql.DB {
	// This would typically use a test database or mock
	// For now, return nil - tests will need actual database
	return nil
}

func TestNewValidator(t *testing.T) {
	conn := createMockConnection()
	if conn == nil {
		t.Skip("Skipping test - requires database connection")
	}

	validator := NewValidator(conn)
	if validator == nil {
		t.Error("NewValidator returned nil")
	}

	if validator.conn != conn {
		t.Error("Validator connection not set correctly")
	}
}

func TestValidationResult_Summary(t *testing.T) {
	result := &ValidationResult{
		Flavor:             "MariaDB",
		Version:            "10.5.8",
		BinaryLogEnabled:   true,
		Statements: []StatementValidationResult{
			{
				Statement:          "ALTER TABLE users ADD COLUMN email VARCHAR(255)",
				TableName:          "users",
				StorageEngine:      "InnoDB",
				StorageEngineValid: true,
				OnlineDDLPossible:  true,
				OnlineDDLReason:    "Operation supports Online DDL (ALGORITHM=INPLACE, LOCK=NONE)",
				PTOSCPossible:      false,
				PTOSCReason:        "",
				TableRows:          1000,
				TableSizeMB:        5.5,
				Warnings:            []string{},
				Errors:              []string{},
			},
		},
		ValidationWarnings: []string{},
		ValidationErrors:  []string{},
	}

	summary := result.Summary()
	if summary == "" {
		t.Error("Summary is empty")
	}

	if !contains(summary, "MariaDB") {
		t.Error("Summary should contain database flavor")
	}

	if !contains(summary, "10.5.8") {
		t.Error("Summary should contain version")
	}

	if !contains(summary, "ALTER TABLE users") {
		t.Error("Summary should contain DDL statement")
	}

	result.ValidationWarnings = append(result.ValidationWarnings, "Binary log is not enabled")
	summary = result.Summary()
	if !contains(summary, "Warnings:") {
		t.Error("Summary should contain warnings section")
	}
}

func TestParseTableName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantDB   string
		wantTbl  string
		wantErr  bool
	}{
		{
			name:    "full table name",
			input:   "mydb.mytable",
			wantDB:  "mydb",
			wantTbl: "mytable",
			wantErr: false,
		},
		{
			name:    "table name only",
			input:   "mytable",
			wantDB:  "", // Will be determined from current database
			wantTbl: "mytable",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This would need a database connection
			t.Skip("Skipping test - requires database connection")
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || 
		(len(s) > len(substr) && (s[:len(substr)] == substr || 
		 s[len(s)-len(substr):] == substr || 
		 containsInner(s, substr))))
}

func containsInner(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

