# Task Automator

A Golang tool for executing schema changes against MySQL/MariaDB servers with validation and safe execution phases.

## Features

### Phase 1: Validation
- Connects to MySQL/MariaDB database server
- Retrieves database flavor and version
- Validates binary log is enabled
- Checks storage engine (must be InnoDB for Online DDL)
- Validates if Online DDL (ALGORITHM=INPLACE, LOCK=NONE) is possible
- Checks table size (rows and megabytes)

### Phase 2: Execution
- Performs table backups (mysqldump, xtrabackup)
- Executes schema changes using pt-online-schema-change (with dry-run validation)
- Falls back to Online DDL when pt-online-schema-change is not used
- Gracefully handles operations that don't support Online DDL

## Installation

```bash
go mod download
go build -o task-automator ./cmd/task-automator
```

## Configuration

The application uses the main mysqlconfigurer configuration file (`releem.conf`). The following HCL configuration fields are available for task-automator:

- `backup_dir`: Directory for backups (default: `/tmp/backups`)
- `ptosc_path`: Path to pt-online-schema-change binary (default: `pt-online-schema-change`)
- `mysqldump_path`: Path to mysqldump binary (default: `mysqldump`)
- `xtrabackup_path`: Path to xtrabackup binary (default: `xtrabackup`)
- `backup_space_buffer`: Disk space buffer percentage (default: `20.0`)

### Example Configuration

Add to your `releem.conf` file:

```hcl
backup_dir = "/var/backups/mysql"
ptosc_path = "/usr/local/bin/pt-online-schema-change"
mysqldump_path = "/usr/bin/mysqldump"
xtrabackup_path = "/usr/bin/xtrabackup"
backup_space_buffer = 20.0
```

The configuration file path can be specified via the `RELEEM_CONFIG` environment variable, or defaults to `/opt/releem/releem.conf`.

## Usage

### Phase 1: Validation

```bash
export MYSQL_DSN="user:password@tcp(localhost:3306)/database"
./task-automator validate table_name
```

Or use the Go API directly:

```go
conn, _ := db.Connect(dsn)
validator := phase1.NewValidator(conn)
result, err := validator.Validate("mydb.mytable")
fmt.Print(result.Summary())
```

### Phase 2: Execution

```bash
./task-automator execute table_name "ALTER TABLE table_name ADD COLUMN newcol INT" mysqldump false
```

Or use the Go API:

```go
executor := phase2.NewExecutor(conn)
options := phase2.ExecuteOptions{
    SQL:                    "ALTER TABLE mytable ADD COLUMN newcol INT",
    TableName:              "mydb.mytable",
    DSN:                    dsn,
    BackupMethod:           phase2.BackupMysqldump,
    UsePTOnlineSchemaChange: false,
    BackupDir:              "/tmp/backups",
}
result, err := executor.Execute(options)
```

## Backup Methods

- `none`: No backup performed
- `mysqldump`: Backup using mysqldump (requires mysqldump in PATH)
- `xtrabackup`: Backup using xtrabackup (not fully implemented)

## Architecture

The code follows DRY, KISS, and YAGNI principles:

- **Modular design**: Separate packages for database connection, Phase 1 validation, and Phase 2 execution
- **Reusable utilities**: Table name parsing, ALTER statement extraction, Online DDL validation
- **Clear separation**: Validation phase does not execute changes, execution phase handles all execution concerns

## Testing

Run tests with:

```bash
go test ./...
```

Tests require a database connection for integration tests. Unit tests for utilities are included and run without database.

## Project Structure

```
task-automator/
├── cmd/
│   └── task-automator/
│       └── main.go          # Main CLI application
├── internal/
│   ├── db/
│   │   ├── connection.go    # Database connection utilities
│   │   └── connection_test.go
│   ├── phase1/
│   │   ├── validation.go    # Phase 1 validation logic
│   │   ├── summary.go       # Validation result summary
│   │   └── validation_test.go
│   └── phase2/
│       ├── execution.go      # Phase 2 execution logic
│       ├── utils.go         # Utility functions
│       ├── execution_test.go
│       └── utils_test.go
├── go.mod
└── README.md
```

## Requirements

- Go 1.21+
- MySQL 5.7+ or MariaDB 10.2+
- For pt-online-schema-change: Percona Toolkit installed
- For mysqldump backups: mysqldump in PATH

## License

This project is provided as-is for educational and development purposes.


