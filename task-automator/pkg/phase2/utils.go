package phase2

import (
	"strings"
)

// TableInfo represents table information
type TableInfo struct {
	Database string
	Table    string
}

// ParseTableName parses a table name that may include database name
func ParseTableName(tableName string, getCurrentDB func() (string, error)) (TableInfo, error) {
	parts := strings.Split(tableName, ".")
	if len(parts) == 2 {
		return TableInfo{
			Database: parts[0],
			Table:    parts[1],
		}, nil
	}

	// Get current database
	db, err := getCurrentDB()
	if err != nil {
		return TableInfo{}, err
	}

	return TableInfo{
		Database: db,
		Table:    tableName,
	}, nil
}

// ExtractAlterStatement extracts the ALTER statement part from SQL
func ExtractAlterStatement(sql string) string {
	sql = strings.TrimSpace(sql)
	
	// Remove "ALTER TABLE" prefix and table name
	upperSQL := strings.ToUpper(sql)
	if strings.HasPrefix(upperSQL, "ALTER TABLE") {
		// Find the first opening parenthesis or keyword after table name
		parts := strings.Fields(sql)
		if len(parts) >= 3 {
			// Find where ALTER statement actually starts (after table name)
			// This is simplified - a real parser would be more sophisticated
			startIdx := 3
			for i := 3; i < len(parts); i++ {
				if strings.HasPrefix(strings.ToUpper(parts[i]), "ADD") ||
					strings.HasPrefix(strings.ToUpper(parts[i]), "DROP") ||
					strings.HasPrefix(strings.ToUpper(parts[i]), "MODIFY") ||
					strings.HasPrefix(strings.ToUpper(parts[i]), "CHANGE") ||
					strings.HasPrefix(strings.ToUpper(parts[i]), "ALTER") {
					startIdx = i
					break
				}
			}
			return strings.Join(parts[startIdx:], " ")
		}
	}
	
	return sql
}

// CanUseOnlineDDL checks if the SQL statement can use Online DDL
// This is a simplified check - MySQL/MariaDB would actually validate this
func CanUseOnlineDDL(sql string) bool {
	upperSQL := strings.ToUpper(sql)
	
	// Some operations always require copy
	// Note: MODIFY can be used with or without COLUMN keyword
	notSupported := []string{
		"ADD FULLTEXT",
		"ADD SPATIAL",
		"DROP PRIMARY KEY",
		"MODIFY COLUMN",
		"MODIFY ", // MODIFY without COLUMN (with space to avoid false matches)
		"CHANGE COLUMN",
		"CHANGE ", // CHANGE without COLUMN
	}
	
	for _, op := range notSupported {
		if strings.Contains(upperSQL, op) {
			return false
		}
	}
	
	return true
}


