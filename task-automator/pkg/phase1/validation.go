package phase1

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/Releem/mysqlconfigurer/task-automator/pkg/phase2"
)

// StatementValidationResult represents validation results for a single DDL statement
type StatementValidationResult struct {
	Statement              string
	TableName              string
	StorageEngine          string
	StorageEngineValid     bool
	OnlineDDLPossible      bool
	OnlineDDLReason        string
	PTOSCPossible          bool
	PTOSCReason            string
	TableRows              int64
	TableSizeMB            float64
	Errors                 []string
	Warnings               []string
}

// ValidationResult represents the results of Phase 1 validation
type ValidationResult struct {
	Flavor              string
	Version             string
	BinaryLogEnabled    bool
	Statements          []StatementValidationResult
	ValidationErrors    []string
	ValidationWarnings  []string
}

// Validator performs Phase 1 validation checks
type Validator struct {
	conn *sql.DB
}

// NewValidator creates a new validator instance
func NewValidator(conn *sql.DB) *Validator {
	return &Validator{conn: conn}
}

// ValidateStatements validates one or more DDL statements
func (v *Validator) ValidateStatements(ddlStatements []string) (*ValidationResult, error) {
	result := &ValidationResult{
		Statements:        []StatementValidationResult{},
		ValidationErrors:  []string{},
		ValidationWarnings: []string{},
	}

	// 1.2. Retrieve flavor and version
	if err := v.checkFlavorAndVersion(result); err != nil {
		return nil, fmt.Errorf("failed to check flavor and version: %w", err)
	}

	// 1.3. Validate binary log is enabled
	if err := v.checkBinaryLog(result); err != nil {
		return nil, fmt.Errorf("failed to check binary log: %w", err)
	}

	// Validate each DDL statement
	for _, ddl := range ddlStatements {
		stmtResult, err := v.validateStatement(ddl)
		if err != nil {
			result.ValidationErrors = append(result.ValidationErrors, 
				fmt.Sprintf("Failed to validate statement '%s': %v", ddl, err))
			continue
		}
		result.Statements = append(result.Statements, *stmtResult)
	}

	return result, nil
}

// Validate performs all Phase 1 validation checks (legacy method for backward compatibility)
func (v *Validator) Validate(tableName string) (*ValidationResult, error) {
	// This is kept for backward compatibility but is deprecated
	// In the future, this could be removed
	result := &ValidationResult{
		Statements:        []StatementValidationResult{},
		ValidationErrors:  []string{},
		ValidationWarnings: []string{},
	}

	if err := v.checkFlavorAndVersion(result); err != nil {
		return nil, fmt.Errorf("failed to check flavor and version: %w", err)
	}

	if err := v.checkBinaryLog(result); err != nil {
		return nil, fmt.Errorf("failed to check binary log: %w", err)
	}

	// For backward compatibility, create a simple statement result
	stmtResult := StatementValidationResult{
		Statement: fmt.Sprintf("ALTER TABLE %s ...", tableName),
		TableName: tableName,
	}

	// Check storage engine and table size
	if err := v.checkStorageEngineForTable(tableName, &stmtResult); err != nil {
		return nil, fmt.Errorf("failed to check storage engine: %w", err)
	}

	if err := v.checkTableSizeForTable(tableName, &stmtResult); err != nil {
		return nil, fmt.Errorf("failed to check table size: %w", err)
	}

	if err := v.validateOnlineDDLForStatement("", tableName, &stmtResult); err != nil {
		return nil, fmt.Errorf("failed to validate Online DDL: %w", err)
	}

	result.Statements = append(result.Statements, stmtResult)

	return result, nil
}

func (v *Validator) validateStatement(ddl string) (*StatementValidationResult, error) {
	stmtResult := &StatementValidationResult{
		Statement: ddl,
		Errors:    []string{},
		Warnings:  []string{},
	}

	// Parse DDL to extract table name
	tableName, err := v.extractTableNameFromDDL(ddl)
	if err != nil {
		stmtResult.Errors = append(stmtResult.Errors, err.Error())
		return stmtResult, nil // Continue validation even if table name parsing fails
	}
	stmtResult.TableName = tableName

	// Check storage engine
	if err := v.checkStorageEngineForTable(tableName, stmtResult); err != nil {
		return nil, err
	}

	// Check table size
	if err := v.checkTableSizeForTable(tableName, stmtResult); err != nil {
		return nil, err
	}

	// Validate Online DDL support for this specific statement
	if err := v.validateOnlineDDLForStatement(ddl, tableName, stmtResult); err != nil {
		return nil, err
	}

	// If Online DDL is not possible, check pt-online-schema-change prerequisites
	if !stmtResult.OnlineDDLPossible {
		if err := v.validatePTOSCPrerequisites(tableName, ddl, stmtResult); err != nil {
			return nil, err
		}
	}

	return stmtResult, nil
}

func (v *Validator) extractTableNameFromDDL(ddl string) (string, error) {
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

func (v *Validator) checkStorageEngineForTable(tableName string, stmtResult *StatementValidationResult) error {
	// Parse table name to get database and table
	parts := strings.Split(tableName, ".")
	var dbName, tblName string
	if len(parts) == 2 {
		dbName = parts[0]
		tblName = parts[1]
	} else {
		// Get current database
		err := v.conn.QueryRow("SELECT DATABASE()").Scan(&dbName)
		if err != nil {
			return err
		}
		tblName = tableName
	}

	engine, err := v.getStorageEngine(dbName, tblName)
	if err != nil {
		if err == sql.ErrNoRows {
			stmtResult.Errors = append(stmtResult.Errors, "Table does not exist")
			return nil
		}
		return err
	}

	stmtResult.StorageEngine = engine
	stmtResult.StorageEngineValid = strings.ToUpper(engine) == "INNODB"

	if !stmtResult.StorageEngineValid {
		stmtResult.Warnings = append(stmtResult.Warnings, 
			fmt.Sprintf("Storage engine is %s, not InnoDB", engine))
	}

	return nil
}

func (v *Validator) checkTableSizeForTable(tableName string, stmtResult *StatementValidationResult) error {
	parts := strings.Split(tableName, ".")
	var dbName, tblName string
	if len(parts) == 2 {
		dbName = parts[0]
		tblName = parts[1]
	} else {
		err := v.conn.QueryRow("SELECT DATABASE()").Scan(&dbName)
		if err != nil {
			return err
		}
		tblName = tableName
	}

	// Get row count
	var rows int64
	err := v.conn.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM `%s`.`%s`", dbName, tblName)).Scan(&rows)
	if err != nil {
		// Table might not exist, which is already handled above
		return nil
	}
	stmtResult.TableRows = rows

	// Get table size in MB
	var dataLength, indexLength sql.NullInt64
	err = v.conn.QueryRow(`
		SELECT DATA_LENGTH, INDEX_LENGTH 
		FROM information_schema.TABLES 
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
	`, dbName, tblName).Scan(&dataLength, &indexLength)
	if err != nil {
		return err
	}

	var totalBytes int64
	if dataLength.Valid {
		totalBytes += dataLength.Int64
	}
	if indexLength.Valid {
		totalBytes += indexLength.Int64
	}

	stmtResult.TableSizeMB = float64(totalBytes) / (1024 * 1024)

	return nil
}

func (v *Validator) validateOnlineDDLForStatement(ddl, tableName string, stmtResult *StatementValidationResult) error {
	// First check if table exists and is InnoDB
	if !stmtResult.StorageEngineValid {
		stmtResult.OnlineDDLPossible = false
		stmtResult.OnlineDDLReason = "Storage engine is not InnoDB - Online DDL requires InnoDB"
		return nil
	}

	// Check if the specific operation supports Online DDL
	if ddl != "" {
		// Use the utility function from phase2 to check if operation supports Online DDL
		canUseOnlineDDL := phase2.CanUseOnlineDDL(ddl)
		
		// Also validate by attempting to parse the ALTER statement
		// MySQL/MariaDB validates this, so we need to try executing it with validation only
		// Since we can't actually run it, we use pattern matching
		
		if !canUseOnlineDDL {
			stmtResult.OnlineDDLPossible = false
			stmtResult.OnlineDDLReason = "Operation does not support ALGORITHM=INPLACE and LOCK=NONE"
			// PTOSC check will be done in validateStatement after this returns
			return nil
		}

		// For operations that might support Online DDL, we check using MySQL's validation
		// by attempting EXPLAIN or using MySQL 8.0's validation
		// For now, we trust the pattern matching and note that actual validation happens at execution
		
		// Validate Online DDL support based on operation type
		// We check if the operation supports ALGORITHM=INPLACE and LOCK=NONE
		// Final validation happens at execution time, but we can provide initial assessment
		stmtResult.OnlineDDLPossible = canUseOnlineDDL
		if canUseOnlineDDL {
			stmtResult.OnlineDDLReason = "Operation supports Online DDL (ALGORITHM=INPLACE, LOCK=NONE)"
		} else {
			stmtResult.OnlineDDLReason = "Operation does not support Online DDL (ALGORITHM=INPLACE, LOCK=NONE) - will use table lock"
		}
	} else {
		// No DDL provided, just check if table supports it
		stmtResult.OnlineDDLPossible = stmtResult.StorageEngineValid
		if stmtResult.OnlineDDLPossible {
			stmtResult.OnlineDDLReason = "Table supports Online DDL (InnoDB engine)"
		}
	}

	return nil
}

// validatePTOSCPrerequisites checks if pt-online-schema-change can be used as an alternative
func (v *Validator) validatePTOSCPrerequisites(tableName, ddl string, stmtResult *StatementValidationResult) error {
	stmtResult.PTOSCPossible = true
	var reasons []string

	// Parse table name to get database and table
	parts := strings.Split(tableName, ".")
	var dbName, tblName string
	if len(parts) == 2 {
		dbName = parts[0]
		tblName = parts[1]
	} else {
		err := v.conn.QueryRow("SELECT DATABASE()").Scan(&dbName)
		if err != nil {
			return err
		}
		tblName = tableName
	}

	// Note: Binary log check is done at ValidationResult level
	// Check binary log status for pt-osc requirements
	var logBinVar sql.NullString
	var logBinValue sql.NullString
	err2 := v.conn.QueryRow("SHOW VARIABLES LIKE 'log_bin'").Scan(&logBinVar, &logBinValue)
	if err2 == nil {
		// Check if binary log is enabled
		if !logBinValue.Valid || logBinValue.String == "OFF" || logBinValue.String == "0" {
			// For MariaDB, also check log_bin_basename
			var logBinBasenameVar sql.NullString
			var logBinBasenameValue sql.NullString
			err3 := v.conn.QueryRow("SHOW VARIABLES LIKE 'log_bin_basename'").Scan(&logBinBasenameVar, &logBinBasenameValue)
			if err3 != nil || !logBinBasenameValue.Valid || logBinBasenameValue.String == "" {
				stmtResult.PTOSCPossible = false
				reasons = append(reasons, "Binary log must be enabled for pt-online-schema-change")
			}
		}
	}

	// Check 2: Table must have a PRIMARY KEY or UNIQUE NOT NULL key
	hasPrimaryKey, err := v.checkPrimaryOrUniqueKey(dbName, tblName)
	if err != nil {
		stmtResult.Warnings = append(stmtResult.Warnings, 
			fmt.Sprintf("Could not verify PRIMARY KEY/UNIQUE key: %v", err))
	} else if !hasPrimaryKey {
		stmtResult.PTOSCPossible = false
		reasons = append(reasons, "Table must have a PRIMARY KEY or UNIQUE NOT NULL key")
	}

	// Check 3: Foreign keys - pt-osc can work with foreign keys but warns
	hasForeignKeys, err := v.checkForeignKeys(dbName, tblName)
	if err == nil && hasForeignKeys {
		stmtResult.Warnings = append(stmtResult.Warnings,
			"Table has foreign keys - pt-online-schema-change will require additional considerations")
	}

	// Check 4: Triggers - pt-osc can work with triggers but needs --preserve-triggers
	hasTriggers, err := v.checkTriggers(dbName, tblName)
	if err == nil && hasTriggers {
		stmtResult.Warnings = append(stmtResult.Warnings,
			"Table has triggers - pt-online-schema-change requires --preserve-triggers flag")
	}

	if len(reasons) > 0 {
		stmtResult.PTOSCReason = strings.Join(reasons, "; ")
	} else if stmtResult.PTOSCPossible {
		stmtResult.PTOSCReason = "pt-online-schema-change can be used as an alternative (tool path should be configured)"
	}

	return nil
}

func (v *Validator) checkPrimaryOrUniqueKey(dbName, tableName string) (bool, error) {
	// Check for PRIMARY KEY
	var pkCount int
	err := v.conn.QueryRow(`
		SELECT COUNT(*) 
		FROM information_schema.KEY_COLUMN_USAGE 
		WHERE TABLE_SCHEMA = ? 
		AND TABLE_NAME = ? 
		AND CONSTRAINT_NAME = 'PRIMARY'
	`, dbName, tableName).Scan(&pkCount)
	if err != nil {
		return false, err
	}

	if pkCount > 0 {
		return true, nil
	}

	// Check for UNIQUE NOT NULL key
	var uniqueNotNullCount int
	err = v.conn.QueryRow(`
		SELECT COUNT(DISTINCT kcu.CONSTRAINT_NAME)
		FROM information_schema.KEY_COLUMN_USAGE kcu
		JOIN information_schema.TABLE_CONSTRAINTS tc 
			ON kcu.CONSTRAINT_NAME = tc.CONSTRAINT_NAME 
			AND kcu.TABLE_SCHEMA = tc.TABLE_SCHEMA
			AND kcu.TABLE_NAME = tc.TABLE_NAME
		JOIN information_schema.COLUMNS c 
			ON kcu.TABLE_SCHEMA = c.TABLE_SCHEMA
			AND kcu.TABLE_NAME = c.TABLE_NAME
			AND kcu.COLUMN_NAME = c.COLUMN_NAME
		WHERE kcu.TABLE_SCHEMA = ?
		AND kcu.TABLE_NAME = ?
		AND tc.CONSTRAINT_TYPE = 'UNIQUE'
		AND c.IS_NULLABLE = 'NO'
	`, dbName, tableName).Scan(&uniqueNotNullCount)
	if err != nil {
		return false, err
	}

	return uniqueNotNullCount > 0, nil
}

func (v *Validator) checkForeignKeys(dbName, tableName string) (bool, error) {
	var fkCount int
	err := v.conn.QueryRow(`
		SELECT COUNT(*) 
		FROM information_schema.KEY_COLUMN_USAGE 
		WHERE TABLE_SCHEMA = ? 
		AND TABLE_NAME = ? 
		AND REFERENCED_TABLE_NAME IS NOT NULL
	`, dbName, tableName).Scan(&fkCount)
	if err != nil {
		return false, err
	}
	return fkCount > 0, nil
}

func (v *Validator) checkTriggers(dbName, tableName string) (bool, error) {
	var triggerCount int
	err := v.conn.QueryRow(`
		SELECT COUNT(*) 
		FROM information_schema.TRIGGERS 
		WHERE TRIGGER_SCHEMA = ? 
		AND EVENT_OBJECT_TABLE = ?
	`, dbName, tableName).Scan(&triggerCount)
	if err != nil {
		return false, err
	}
	return triggerCount > 0, nil
}

func (v *Validator) checkFlavorAndVersion(result *ValidationResult) error {
	var version string
	err := v.conn.QueryRow("SELECT VERSION()").Scan(&version)
	if err != nil {
		return err
	}

	result.Version = version

	// Determine flavor
	var flavor string
	err = v.conn.QueryRow("SELECT @@version_comment").Scan(&flavor)
	if err != nil {
		return err
	}

	if strings.Contains(strings.ToLower(flavor), "mariadb") {
		result.Flavor = "MariaDB"
	} else {
		result.Flavor = "MySQL"
	}

	return nil
}

func (v *Validator) checkBinaryLog(result *ValidationResult) error {
	var logBin string
	err := v.conn.QueryRow("SELECT @@log_bin").Scan(&logBin)
	if err != nil {
		return err
	}

	result.BinaryLogEnabled = logBin == "1" || strings.ToLower(logBin) == "on"

	if !result.BinaryLogEnabled {
		result.ValidationWarnings = append(result.ValidationWarnings, "Binary log is not enabled")
	}

	return nil
}

func (v *Validator) getStorageEngine(dbName, tableName string) (string, error) {
	var engine string
	query := `
		SELECT ENGINE 
		FROM information_schema.TABLES 
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
	`
	err := v.conn.QueryRow(query, dbName, tableName).Scan(&engine)
	if err != nil {
		return "", err
	}
	return engine, nil
}
