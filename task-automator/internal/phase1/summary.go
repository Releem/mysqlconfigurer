package phase1

import (
	"fmt"
	"strings"
)

// Summary returns a human-readable summary of the validation results
func (r *ValidationResult) Summary() string {
	var summary strings.Builder

	summary.WriteString("=== Phase 1 Validation Results ===\n")
	summary.WriteString(fmt.Sprintf("Database Flavor: %s\n", r.Flavor))
	summary.WriteString(fmt.Sprintf("Version: %s\n", r.Version))
	summary.WriteString(fmt.Sprintf("Binary Log Enabled: %v\n", r.BinaryLogEnabled))
	summary.WriteString(fmt.Sprintf("Number of Statements: %d\n\n", len(r.Statements)))

	// Summary for each statement
	for i, stmt := range r.Statements {
		summary.WriteString(fmt.Sprintf("--- Statement %d ---\n", i+1))
		summary.WriteString(fmt.Sprintf("DDL: %s\n", stmt.Statement))
		summary.WriteString(fmt.Sprintf("Table: %s\n", stmt.TableName))
		summary.WriteString(fmt.Sprintf("Storage Engine: %s\n", stmt.StorageEngine))
		summary.WriteString(fmt.Sprintf("Storage Engine Valid (InnoDB): %v\n", stmt.StorageEngineValid))
		summary.WriteString(fmt.Sprintf("Online DDL Possible: %v\n", stmt.OnlineDDLPossible))
		if stmt.OnlineDDLReason != "" {
			summary.WriteString(fmt.Sprintf("Online DDL Reason: %s\n", stmt.OnlineDDLReason))
		}
		if !stmt.OnlineDDLPossible {
			summary.WriteString(fmt.Sprintf("pt-online-schema-change Possible: %v\n", stmt.PTOSCPossible))
			if stmt.PTOSCReason != "" {
				summary.WriteString(fmt.Sprintf("pt-online-schema-change Reason: %s\n", stmt.PTOSCReason))
			}
		}
		if stmt.TableRows > 0 {
			summary.WriteString(fmt.Sprintf("Table Rows: %d\n", stmt.TableRows))
		}
		if stmt.TableSizeMB > 0 {
			summary.WriteString(fmt.Sprintf("Table Size (MB): %.2f\n", stmt.TableSizeMB))
		}

		if len(stmt.Warnings) > 0 {
			summary.WriteString("Warnings:\n")
			for _, warning := range stmt.Warnings {
				summary.WriteString(fmt.Sprintf("  - %s\n", warning))
			}
		}

		if len(stmt.Errors) > 0 {
			summary.WriteString("Errors:\n")
			for _, err := range stmt.Errors {
				summary.WriteString(fmt.Sprintf("  - %s\n", err))
			}
		}

		summary.WriteString("\n")
	}

	if len(r.ValidationWarnings) > 0 {
		summary.WriteString("General Warnings:\n")
		for _, warning := range r.ValidationWarnings {
			summary.WriteString(fmt.Sprintf("  - %s\n", warning))
		}
	}

	if len(r.ValidationErrors) > 0 {
		summary.WriteString("General Errors:\n")
		for _, err := range r.ValidationErrors {
			summary.WriteString(fmt.Sprintf("  - %s\n", err))
		}
	}

	return summary.String()
}


