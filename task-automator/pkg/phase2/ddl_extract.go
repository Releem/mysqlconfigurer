package phase2

import (
	"fmt"
	"regexp"
	"strings"
)

// ExtractTableNameFromDDL extracts the table name from a DDL statement
// This function is similar to the one in phase1 but kept here for phase2 use
func ExtractTableNameFromDDL(ddl string) (string, error) {
	ddl = strings.TrimSpace(ddl)
	upperDDL := strings.ToUpper(ddl)

	// Check if it's an ALTER TABLE statement
	if !strings.HasPrefix(upperDDL, "ALTER TABLE") {
		return "", fmt.Errorf("statement is not an ALTER TABLE statement")
	}

	// Remove ALTER TABLE prefix
	ddl = strings.TrimSpace(ddl[12:]) // Remove "ALTER TABLE"
	
	// Extract table name (can be `db.table` or `table`)
	// Handle backticks
	ddl = strings.TrimSpace(ddl)
	
	// Match table name pattern: `db`.`table` or `table` or db.table or table
	re := regexp.MustCompile(`^(` + "(`[^`]+`\\.)?`[^`]+`|" + "[^\\s`\\.]+\\.?[^\\s`\\.]+" + ")")
	
	matches := re.FindStringSubmatch(ddl)
	if len(matches) > 1 && matches[1] != "" {
		tableName := matches[1]
		// Remove all backticks
		tableName = strings.ReplaceAll(tableName, "`", "")
		return tableName, nil
	}

	// Fallback: split by space and take first token
	parts := strings.Fields(ddl)
	if len(parts) > 0 {
		tableName := strings.Trim(parts[0], "`")
		return tableName, nil
	}

	return "", fmt.Errorf("could not extract table name from DDL statement")
}

