package phase2

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/Releem/mysqlconfigurer/config"
)

// BackupMethod represents the type of backup to perform
type BackupMethod string

const (
	BackupNone      BackupMethod = "none"
	BackupMysqldump BackupMethod = "mysqldump"
	BackupXtrabackup BackupMethod = "xtrabackup"
)

// Executor handles Phase 2 schema change execution
type Executor struct {
	conn *sql.DB
}

// NewExecutor creates a new executor instance
func NewExecutor(conn *sql.DB) *Executor {
	return &Executor{conn: conn}
}

// ExecuteOptions contains options for schema change execution
type ExecuteOptions struct {
	SQL                    string
	TableName              string
	DSN                    string // Database connection string (required for backups and pt-osc)
	BackupMethod           BackupMethod
	UsePTOnlineSchemaChange bool
	Config                 *config.Config // Configuration with paths and directories
	Debug                  bool           // Enable debug output (print commands and outputs)
}

// ExecuteResult represents the result of Phase 2 execution
type ExecuteResult struct {
	BackupPerformed    bool
	BackupPath         string
	ChangeExecuted     bool
	MethodUsed         string
	Warnings           []string
	Errors             []string
}

// Execute performs Phase 2 schema change execution
func (e *Executor) Execute(options ExecuteOptions) (*ExecuteResult, error) {
	result := &ExecuteResult{
		Warnings: []string{},
		Errors:   []string{},
	}

	// Extract table name from SQL if not provided
	if options.TableName == "" {
		tableName, err := ExtractTableNameFromDDL(options.SQL)
		if err != nil {
			return nil, fmt.Errorf("failed to extract table name from SQL: %w", err)
		}
		options.TableName = tableName
	}

	// 2.1. Perform backup if specified
	if options.BackupMethod != BackupNone {
		backupPath, err := e.performBackup(options)
		if err != nil {
			return nil, fmt.Errorf("backup failed: %w", err)
		}
		result.BackupPerformed = true
		result.BackupPath = backupPath
	}

	// 2.2. Execute change using pt-online-schema-change if specified
	if options.UsePTOnlineSchemaChange {
		if err := e.executeWithPTOSC(options); err != nil {
			return nil, fmt.Errorf("pt-online-schema-change execution failed: %w", err)
		}
		result.ChangeExecuted = true
		result.MethodUsed = "pt-online-schema-change"
		return result, nil
	}

	// 2.3. Execute using Online DDL
	if err := e.executeWithOnlineDDL(options, result); err != nil {
		// 2.4. Fallback to regular ALTER if Online DDL fails
		if err := e.executeWithRegularAlter(options, result); err != nil {
			return nil, fmt.Errorf("schema change execution failed: %w", err)
		}
		result.ChangeExecuted = true
		result.MethodUsed = "regular ALTER (with table lock warning)"
		return result, nil
	}

	result.ChangeExecuted = true
	result.MethodUsed = "Online DDL"
	return result, nil
}

func (e *Executor) performBackup(options ExecuteOptions) (string, error) {
	// Check disk space before performing backup
	if err := e.checkDiskSpace(options); err != nil {
		return "", err
	}

	switch options.BackupMethod {
	case BackupMysqldump:
		return e.backupWithMysqldump(options)
	case BackupXtrabackup:
		return e.backupWithXtrabackup(options)
	default:
		return "", fmt.Errorf("unsupported backup method: %s", options.BackupMethod)
	}
}

func (e *Executor) backupWithMysqldump(options ExecuteOptions) (string, error) {
	// if options.DSN == "" {
	// 	return "", fmt.Errorf("DSN is required for backup")
	// }

	if options.Config == nil {
		return "", fmt.Errorf("config is required for backup")
	}

	host := options.Config.MysqlHost
	port := options.Config.MysqlPort
	user := options.Config.MysqlUser
	password := options.Config.MysqlPassword
	if host == "" {
		return "", fmt.Errorf("could not parse DSN")
	}

	// Parse table name
	tableInfo, err := ParseTableName(options.TableName, func() (string, error) {
		var db string
		err := e.conn.QueryRow("SELECT DATABASE()").Scan(&db)
		return db, err
	})
	if err != nil {
		return "", err
	}

	// Use config values
	mysqldump := options.Config.MysqldumpPath
	if mysqldump == "" {
		mysqldump = "mysqldump"
	}

	// Ensure backup directory exists
	if err := os.MkdirAll(options.Config.BackupDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Generate timestamp prefix in YYMMDDHHMMSS format
	timestamp := time.Now().Format("060102150405")
	backupPath := fmt.Sprintf("%s/%s_%s_%s.sql", options.Config.BackupDir, timestamp, tableInfo.Database, tableInfo.Table)

	args := []string{
		"-h", host,
		"-P", port,
		"-u", user,
		"-p" + password,
		tableInfo.Database,
		tableInfo.Table,
		"--single-transaction",
		"--quick",
		"--lock-tables=false",
		"-r", backupPath,
	}

	cmd := exec.Command(mysqldump, args...)
	
	if options.Debug {
		// Mask password in debug output
		safeArgs := make([]string, len(args))
		copy(safeArgs, args)
		// Find and mask password argument (-p followed by password)
		for i, arg := range safeArgs {
			if strings.HasPrefix(arg, "-p") && len(arg) > 2 {
				safeArgs[i] = "-p***"
			}
		}
		fmt.Printf("[DEBUG] mysqldump command: %s\n", mysqldump)
		fmt.Printf("[DEBUG] mysqldump args: %s\n", strings.Join(safeArgs, " "))
	}

	output, err := cmd.CombinedOutput()
	if options.Debug {
		if len(output) > 0 {
			fmt.Printf("[DEBUG] mysqldump output:\n%s\n", string(output))
		}
	}
	
	if err != nil {
		if options.Debug {
			fmt.Printf("[DEBUG] mysqldump error: %v\n", err)
		}
		return "", fmt.Errorf("mysqldump failed: %w", err)
	}

	return backupPath, nil
}

func (e *Executor) backupWithXtrabackup(options ExecuteOptions) (string, error) {
	// if options.DSN == "" {
	// 	return "", fmt.Errorf("DSN is required for backup")
	// }

	if options.Config == nil {
		return "", fmt.Errorf("config is required for backup")
	}

	// Parse DSN to get connection details

	host := options.Config.MysqlHost
	port := options.Config.MysqlPort
	user := options.Config.MysqlUser
	password := options.Config.MysqlPassword
	// host, port, user, password, _ := e.parseDSN(options.DSN)
	// if host == "" {
	// 	return "", fmt.Errorf("could not parse DSN")
	// }

	// Parse table name
	tableInfo, err := ParseTableName(options.TableName, func() (string, error) {
		var db string
		err := e.conn.QueryRow("SELECT DATABASE()").Scan(&db)
		return db, err
	})
	if err != nil {
		return "", err
	}

	// Use config values
	xtrabackup := options.Config.XtrabackupPath
	if xtrabackup == "" {
		xtrabackup = "xtrabackup"
	}

	// Ensure backup directory exists
	if err := os.MkdirAll(options.Config.BackupDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Generate timestamp prefix in YYMMDDHHMMSS format
	timestamp := time.Now().Format("060102150405")
	// Create a unique backup directory for this table
	backupDir := fmt.Sprintf("%s/%s_xtrabackup_%s_%s", options.Config.BackupDir, timestamp, tableInfo.Database, tableInfo.Table)

	// Step 1: Take backup of the table using --tables option
	// Format: database.table
	tableSpec := fmt.Sprintf("%s.%s", tableInfo.Database, tableInfo.Table)
	
	backupArgs := []string{
		"--backup",
		"--tables=" + tableSpec,
		"--target-dir=" + backupDir,
		"--user=" + user,
		"--password=" + password,
		"--host=" + host,
		"--port=" + port,
	}

	if options.Debug {
		// Mask password in debug output
		safeArgs := make([]string, len(backupArgs))
		copy(safeArgs, backupArgs)
		for i, arg := range safeArgs {
			if strings.HasPrefix(arg, "--password=") {
				safeArgs[i] = "--password=***"
			}
		}
		fmt.Printf("[DEBUG] xtrabackup backup command: %s\n", xtrabackup)
		fmt.Printf("[DEBUG] xtrabackup backup args: %s\n", strings.Join(safeArgs, " "))
	}

	cmd := exec.Command(xtrabackup, backupArgs...)
	output, err := cmd.CombinedOutput()
	if options.Debug {
		if len(output) > 0 {
			fmt.Printf("[DEBUG] xtrabackup backup output:\n%s\n", string(output))
		}
		if err != nil {
			fmt.Printf("[DEBUG] xtrabackup backup error: %v\n", err)
		}
	}

	if err != nil {
		return "", fmt.Errorf("xtrabackup backup failed: %w: %s", err, string(output))
	}

	// Step 2: Prepare the backup with --export option
	// This prepares the backup and exports table metadata for transportable tablespace
	prepareArgs := []string{
		"--prepare",
		"--export",
		"--target-dir=" + backupDir,
	}

	if options.Debug {
		fmt.Printf("[DEBUG] xtrabackup prepare command: %s\n", xtrabackup)
		fmt.Printf("[DEBUG] xtrabackup prepare args: %s\n", strings.Join(prepareArgs, " "))
	}

	cmd = exec.Command(xtrabackup, prepareArgs...)
	output, err = cmd.CombinedOutput()
	if options.Debug {
		if len(output) > 0 {
			fmt.Printf("[DEBUG] xtrabackup prepare output:\n%s\n", string(output))
		}
		if err != nil {
			fmt.Printf("[DEBUG] xtrabackup prepare error: %v\n", err)
		}
	}

	if err != nil {
		return "", fmt.Errorf("xtrabackup prepare failed: %w: %s", err, string(output))
	}

	// Return the backup directory path
	// The table files (.ibd and .cfg) will be in backupDir/database/table.*
	return backupDir, nil
}

// checkDiskSpace checks if there's enough disk space for the backup
func (e *Executor) checkDiskSpace(options ExecuteOptions) error {
	var estimatedSize int64
	
	// Parse table name to get database and table
	tableInfo, err := ParseTableName(options.TableName, func() (string, error) {
		var db string
		err := e.conn.QueryRow("SELECT DATABASE()").Scan(&db)
		return db, err
	})
	if err != nil {
		return fmt.Errorf("failed to parse table name: %w", err)
	}

	// Estimate backup size based on method
	switch options.BackupMethod {
	case BackupMysqldump:
		sizeMB, err := e.estimateMysqldumpSize(tableInfo.Database, tableInfo.Table)
		if err != nil {
			return fmt.Errorf("failed to estimate backup size: %w", err)
		}
		estimatedSize = int64(sizeMB * 1024 * 1024) // Convert MB to bytes
	case BackupXtrabackup:
		// Xtrabackup backs up entire database, so we estimate based on database size
		sizeMB, err := e.estimateXtrabackupSize(tableInfo.Database)
		if err != nil {
			return fmt.Errorf("failed to estimate backup size: %w", err)
		}
		estimatedSize = int64(sizeMB * 1024 * 1024) // Convert MB to bytes
	default:
		return nil // No backup, no space check needed
	}

	// Add buffer percentage to estimated size
	bufferPercent := options.Config.BackupSpaceBuffer
	if bufferPercent == 0 {
		bufferPercent = 20.0 // Fallback to 20% if not configured
	}
	requiredSize := estimatedSize + int64(float64(estimatedSize)*bufferPercent/100.0)

	// Check available space in backup directory
	var stat syscall.Statfs_t
	err = syscall.Statfs(options.Config.BackupDir, &stat)
	if err != nil {
		return fmt.Errorf("failed to check disk space: %w", err)
	}

	// Calculate available space (blocks * block size)
	availableBytes := int64(stat.Bavail) * int64(stat.Bsize)

	if options.Debug {
		fmt.Printf("[DEBUG] Estimated backup size: %.2f MB\n", float64(estimatedSize)/(1024*1024))
		fmt.Printf("[DEBUG] Required space (with %.1f%% buffer): %.2f MB\n", bufferPercent, float64(requiredSize)/(1024*1024))
		fmt.Printf("[DEBUG] Available disk space: %.2f MB\n", float64(availableBytes)/(1024*1024))
	}

	if availableBytes < requiredSize {
		return fmt.Errorf("insufficient disk space: required %.2f MB (with %.1f%% buffer), available %.2f MB",
			float64(requiredSize)/(1024*1024),
			bufferPercent,
			float64(availableBytes)/(1024*1024))
	}

	return nil
}

// estimateMysqldumpSize estimates the size of a mysqldump backup for a specific table
func (e *Executor) estimateMysqldumpSize(dbName, tableName string) (float64, error) {
	var dataLength, indexLength sql.NullInt64
	err := e.conn.QueryRow(`
		SELECT DATA_LENGTH, INDEX_LENGTH 
		FROM information_schema.TABLES 
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
	`, dbName, tableName).Scan(&dataLength, &indexLength)
	if err != nil {
		return 0, err
	}

	var totalBytes int64
	if dataLength.Valid {
		totalBytes += dataLength.Int64
	}
	if indexLength.Valid {
		totalBytes += indexLength.Int64
	}

	if totalBytes == 0 {
		return 0.1, nil // Return a minimal size estimate if table size is 0 or NULL
	}

	// mysqldump typically produces 1.5-2x the table size due to SQL format overhead
	estimatedSizeMB := float64(totalBytes) * 2.0 / (1024 * 1024)
	
	return estimatedSizeMB, nil
}

// estimateXtrabackupSize estimates the size of an xtrabackup for the entire database
func (e *Executor) estimateXtrabackupSize(dbName string) (float64, error) {
	var totalBytes sql.NullInt64
	err := e.conn.QueryRow(`
		SELECT SUM(DATA_LENGTH + INDEX_LENGTH) 
		FROM information_schema.TABLES 
		WHERE TABLE_SCHEMA = ?
	`, dbName).Scan(&totalBytes)
	if err != nil {
		return 0, err
	}

	if !totalBytes.Valid || totalBytes.Int64 == 0 {
		return 0.1, nil // Return a minimal size estimate if database size is 0 or NULL
	}

	// Xtrabackup includes all tables, indexes, and some overhead
	// Estimate ~1.2x the database size
	estimatedSizeMB := float64(totalBytes.Int64) * 1.2 / (1024 * 1024)
	
	return estimatedSizeMB, nil
}

func (e *Executor) executeWithPTOSC(options ExecuteOptions) error {
	// Perform dry-run first
	if err := e.dryRunPTOSC(options); err != nil {
		return fmt.Errorf("pt-online-schema-change dry-run failed: %w", err)
	}

	// Execute actual change
	return e.runPTOSC(options)
}

func (e *Executor) dryRunPTOSC(options ExecuteOptions) error {
	// if options.DSN == "" {
	// 	return fmt.Errorf("DSN is required for pt-online-schema-change")
	// }

	if options.Config == nil {
		return fmt.Errorf("config is required for pt-online-schema-change")
	}

	ptosc := options.Config.PTOSCPath
	if ptosc == "" {
		ptosc = "pt-online-schema-change"
	}

	host := options.Config.MysqlHost
	port := options.Config.MysqlPort
	user := options.Config.MysqlUser
	password := options.Config.MysqlPassword
	if host == "" {
		return fmt.Errorf("could not parse DSN")
	}

	tableInfo, err := ParseTableName(options.TableName, func() (string, error) {
		var db string
		err := e.conn.QueryRow("SELECT DATABASE()").Scan(&db)
		return db, err
	})
	if err != nil {
		return err
	}

	// Extract ALTER statement
	alterSQL := ExtractAlterStatement(options.SQL)
	if alterSQL == "" {
		alterSQL = options.SQL
	}

	args := []string{
		"--dry-run",
		fmt.Sprintf("h=%s,P=%s,u=%s,p=%s,D=%s,t=%s", host, port, user, password, tableInfo.Database, tableInfo.Table),
		fmt.Sprintf("--alter=%s", alterSQL),
	}

	cmd := exec.Command(ptosc, args...)
	
	if options.Debug {
		// Mask password in debug output
		safeArgs := make([]string, len(args))
		copy(safeArgs, args)
		for i, arg := range safeArgs {
			// Mask password in connection string (p=password)
			if strings.Contains(arg, ",p=") {
				parts := strings.Split(arg, ",p=")
				if len(parts) == 2 {
					// Extract everything after p= and before next comma
					rest := strings.Split(parts[1], ",")
					if len(rest) > 0 {
						safeArgs[i] = parts[0] + ",p=***," + strings.Join(rest[1:], ",")
					} else {
						safeArgs[i] = parts[0] + ",p=***"
					}
				} else if strings.HasPrefix(arg, "p=") {
					rest := strings.SplitN(arg, ",", 2)
					safeArgs[i] = "p=***"
					if len(rest) > 1 {
						safeArgs[i] += "," + rest[1]
					}
				}
			}
		}
		fmt.Printf("[DEBUG] pt-online-schema-change dry-run command: %s\n", ptosc)
		fmt.Printf("[DEBUG] pt-online-schema-change dry-run args: %s\n", strings.Join(safeArgs, " "))
	}
	
	output, err := cmd.CombinedOutput()
	if options.Debug {
		fmt.Printf("[DEBUG] pt-online-schema-change dry-run output:\n%s\n", string(output))
		if err != nil {
			fmt.Printf("[DEBUG] pt-online-schema-change dry-run error: %v\n", err)
		}
	}
	
	if err != nil {
		return fmt.Errorf("pt-online-schema-change dry-run failed: %s", string(output))
	}

	return nil
}

func (e *Executor) runPTOSC(options ExecuteOptions) error {
	if options.DSN == "" {
		return fmt.Errorf("DSN is required for pt-online-schema-change")
	}

	if options.Config == nil {
		return fmt.Errorf("config is required for pt-online-schema-change")
	}

	ptosc := options.Config.PTOSCPath
	if ptosc == "" {
		ptosc = "pt-online-schema-change"
	}

	host, port, user, password, _ := e.parseDSN(options.DSN)
	if host == "" {
		return fmt.Errorf("could not parse DSN")
	}

	tableInfo, err := ParseTableName(options.TableName, func() (string, error) {
		var db string
		err := e.conn.QueryRow("SELECT DATABASE()").Scan(&db)
		return db, err
	})
	if err != nil {
		return err
	}

	alterSQL := ExtractAlterStatement(options.SQL)
	if alterSQL == "" {
		alterSQL = options.SQL
	}

	args := []string{
		"--execute",
		fmt.Sprintf("h=%s,P=%s,u=%s,p=%s,D=%s,t=%s", host, port, user, password, tableInfo.Database, tableInfo.Table),
		fmt.Sprintf("--alter=%s", alterSQL),
	}

	cmd := exec.Command(ptosc, args...)
	
	if options.Debug {
		// Mask password in debug output
		safeArgs := make([]string, len(args))
		copy(safeArgs, args)
		for i, arg := range safeArgs {
			// Mask password in connection string (p=password)
			if strings.Contains(arg, ",p=") {
				parts := strings.Split(arg, ",p=")
				if len(parts) == 2 {
					// Extract everything after p= and before next comma
					rest := strings.Split(parts[1], ",")
					if len(rest) > 0 {
						safeArgs[i] = parts[0] + ",p=***," + strings.Join(rest[1:], ",")
					} else {
						safeArgs[i] = parts[0] + ",p=***"
					}
				} else if strings.HasPrefix(arg, "p=") {
					rest := strings.SplitN(arg, ",", 2)
					safeArgs[i] = "p=***"
					if len(rest) > 1 {
						safeArgs[i] += "," + rest[1]
					}
				}
			}
		}
		fmt.Printf("[DEBUG] pt-online-schema-change execute command: %s\n", ptosc)
		fmt.Printf("[DEBUG] pt-online-schema-change execute args: %s\n", strings.Join(safeArgs, " "))
	}
	
	output, err := cmd.CombinedOutput()
	if options.Debug {
		fmt.Printf("[DEBUG] pt-online-schema-change execute output:\n%s\n", string(output))
		if err != nil {
			fmt.Printf("[DEBUG] pt-online-schema-change execute error: %v\n", err)
		}
	}
	
	if err != nil {
		return fmt.Errorf("pt-online-schema-change failed: %s", string(output))
	}

	return nil
}

func (e *Executor) executeWithOnlineDDL(options ExecuteOptions, result *ExecuteResult) error {
	// Parse SQL to check if it already has ALGORITHM=INPLACE and LOCK=NONE
	sql := strings.TrimSpace(options.SQL)
	upperSQL := strings.ToUpper(sql)
	
	hasAlgorithm := strings.Contains(upperSQL, "ALGORITHM=")
	hasLock := strings.Contains(upperSQL, "LOCK=")
	
	// Remove trailing semicolon if present
	sql = strings.TrimSuffix(sql, ";")
	sql = strings.TrimSpace(sql)
	
	if !hasAlgorithm && !hasLock {
		sql += ", ALGORITHM=INPLACE, LOCK=NONE"
	} else if !hasAlgorithm {
		sql += ", ALGORITHM=INPLACE"
	} else if !hasLock {
		sql += ", LOCK=NONE"
	}

	// Execute the ALTER statement
	if options.Debug {
		fmt.Printf("[DEBUG] Executing Online DDL statement:\n%s\n", sql)
	}
	
		_, err := e.conn.Exec(sql)
	if options.Debug {
		if err != nil {
			fmt.Printf("[DEBUG] Online DDL execution error: %v\n", err)
		} else {
			fmt.Printf("[DEBUG] Online DDL execution successful\n")
		}
	}
	
	if err != nil {
		// Check if error is due to Online DDL not being supported
		errStr := err.Error()
		if strings.Contains(errStr, "ALGORITHM=INPLACE") || 
		   strings.Contains(errStr, "LOCK=NONE") ||
		   strings.Contains(errStr, "ALGORITHM=INPLACE is not supported") {
			result.Warnings = append(result.Warnings, 
				"Online DDL not supported for this operation")
			return err
		}
		return err
	}

	return nil
}

func (e *Executor) executeWithRegularAlter(options ExecuteOptions, result *ExecuteResult) error {
	result.Warnings = append(result.Warnings, 
		"Executing schema change without Online DDL - table may be locked during execution")

	if options.Debug {
		fmt.Printf("[DEBUG] Executing regular ALTER statement (without Online DDL):\n%s\n", options.SQL)
	}
	
		_, err := e.conn.Exec(options.SQL)
	if options.Debug {
		if err != nil {
			fmt.Printf("[DEBUG] Regular ALTER execution error: %v\n", err)
		} else {
			fmt.Printf("[DEBUG] Regular ALTER execution successful\n")
		}
	}
	
	return err
}

func (e *Executor) parseDSN(dsn string) (host, port, user, password, database string) {
	// Parse MySQL DSN format: user:password@tcp(host:port)/database
	// This is a simplified parser
	parts := strings.Split(dsn, "@")
	if len(parts) != 2 {
		return "", "", "", "", ""
	}

	userPass := parts[0]
	tcpAndDB := parts[1]

	userPassParts := strings.Split(userPass, ":")
	if len(userPassParts) == 2 {
		user = userPassParts[0]
		password = userPassParts[1]
	} else {
		user = userPass
	}

	// Parse tcp(host:port)/database
	if strings.HasPrefix(tcpAndDB, "tcp(") {
		end := strings.Index(tcpAndDB, ")")
		if end > 0 {
			hostPort := tcpAndDB[4:end]
			hostPortParts := strings.Split(hostPort, ":")
			if len(hostPortParts) == 2 {
				host = hostPortParts[0]
				port = hostPortParts[1]
			} else {
				host = hostPort
				port = "3306"
			}
		}
		dbPart := tcpAndDB[end+1:]
		if strings.HasPrefix(dbPart, "/") {
			database = dbPart[1:]
		}
	}

	if port == "" {
		port = "3306"
	}

	return host, port, user, password, database
}

