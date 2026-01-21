package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/go-sql-driver/mysql"
	logging "github.com/google/logger"
	"github.com/Releem/mysqlconfigurer/config"
	"github.com/Releem/mysqlconfigurer/task-automator/pkg/phase1"
	"github.com/Releem/mysqlconfigurer/task-automator/pkg/phase2"
)

func main() {
	// Example usage of the task-automator
	
	if len(os.Args) < 2 {
		fmt.Println("Usage: ./task-automator <command> [options]")
		fmt.Println("Commands: validate, execute")
		os.Exit(1)
	}

	command := os.Args[1]

	// Load configuration from main config file
	// Default config path or use environment variable
	configPath := os.Getenv("RELEEM_CONFIG")
	if configPath == "" {
		configPath = "/opt/releem/releem.conf"
	}
	
	logger := logging.Init("task-automator", false, false, os.Stderr)
	var cfg *config.Config
	var err error
	cfg, err = config.LoadConfig(configPath, *logger)
	if err != nil {
		// If config file doesn't exist, create a default config
		cfg = &config.Config{
			BackupDir:         "/tmp/backups",
			PTOSCPath:         "pt-online-schema-change",
			MysqldumpPath:     "mysqldump",
			XtrabackupPath:    "xtrabackup",
			BackupSpaceBuffer: 20.0,
		}
		logger.Infof("Using default configuration (config file not found)")
	}

	// Example DSN - replace with actual connection details
	dsn := getDSN()
	
	conn, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer conn.Close()
	
	if err := conn.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	switch command {
	case "validate":
		if len(os.Args) < 3 {
			log.Fatal("Usage: validate <ddl_statement1> [ddl_statement2] ...")
		}
		ddlStatements := os.Args[2:]
		runPhase1(conn, ddlStatements)
		
	case "execute":
		if len(os.Args) < 3 {
			log.Fatal("Usage: execute <sql> [backup_method] [use_ptosc] [--debug|-d]")
		}
		sql := os.Args[2]
		backupMethod := phase2.BackupNone
		usePTOSC := false
		debug := false
		
		// Parse arguments (skip SQL which is args[2])
		nonSQLArgs := []string{}
		for i := 3; i < len(os.Args); i++ {
			arg := os.Args[i]
			if arg == "--debug" || arg == "-d" {
				debug = true
			} else {
				nonSQLArgs = append(nonSQLArgs, arg)
			}
		}
		
		// First non-debug argument is backup method
		if len(nonSQLArgs) > 0 {
			backupMethod = phase2.BackupMethod(nonSQLArgs[0])
		}
		// Second non-debug argument is use_ptosc flag
		if len(nonSQLArgs) > 1 {
			usePTOSC = (nonSQLArgs[1] == "true")
		}
		
		runPhase2(conn, dsn, sql, backupMethod, usePTOSC, debug, cfg)
		
	default:
		log.Fatalf("Unknown command: %s", command)
	}
}

func getDSN() string {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		dsn = "user:password@tcp(localhost:3306)/testdb"
	}
	return dsn
}


func runPhase1(conn *sql.DB, ddlStatements []string) {
	validator := phase1.NewValidator(conn)
	
	result, err := validator.ValidateStatements(ddlStatements)
	if err != nil {
		log.Fatalf("Validation failed: %v", err)
	}
	
	fmt.Print(result.Summary())
}

func runPhase2(conn *sql.DB, dsn, sql string, backupMethod phase2.BackupMethod, usePTOSC bool, debug bool, cfg *config.Config) {
	executor := phase2.NewExecutor(conn)
	
	if debug {
		fmt.Println("[DEBUG] Debug mode enabled")
		fmt.Printf("[DEBUG] SQL statement: %s\n", sql)
		fmt.Printf("[DEBUG] Backup method: %s\n", backupMethod)
		fmt.Printf("[DEBUG] Use pt-online-schema-change: %v\n", usePTOSC)
		// Don't print DSN as it contains password
	}
	
	options := phase2.ExecuteOptions{
		SQL:                    sql,
		TableName:              "", // Will be extracted from SQL
		DSN:                    dsn,
		BackupMethod:           backupMethod,
		UsePTOnlineSchemaChange: usePTOSC,
		Config:                 cfg,
		Debug:                  debug,
	}
	
	result, err := executor.Execute(options)
	if err != nil {
		log.Fatalf("Execution failed: %v", err)
	}
	
	fmt.Printf("Change executed successfully!\n")
	fmt.Printf("Method used: %s\n", result.MethodUsed)
	if result.BackupPerformed {
		fmt.Printf("Backup created at: %s\n", result.BackupPath)
	}
	if len(result.Warnings) > 0 {
		fmt.Println("Warnings:")
		for _, w := range result.Warnings {
			fmt.Printf("  - %s\n", w)
		}
	}
}


