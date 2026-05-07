# Task Type 6: Schema Change Execution

Task type 6 is the Releem Agent path for applying DDL statements that have already been analyzed by the schema-change workflow. The main entry point is `ApplySchemaChanges` in `tasks/tasks.go`; the actual DDL execution lives in `phase2.Executor.Execute`.

## Entry Point

`ProcessTask` receives the task payload from the repeater and marks the task as started before dispatching by `TypeID`. For `TypeID == 6`, it calls:

```go
ApplySchemaChanges(logger, configuration, TaskStruct.Details)
```

The function returns the final task exit code, status, output, and error string. Those values are copied back to `TaskStruct` and reported through the task status repeater.

## Task Details Payload

`TaskStruct.Details` must be a JSON array. Each item represents one DDL statement:

```json
[
  {
    "schema_name": "app",
    "ddl_statement": "ALTER TABLE app.users ADD COLUMN last_seen_at DATETIME NULL",
    "pre_change_bkp": true,
    "analysis_results": {
      "schema_name": "app",
      "table_name": "users",
      "syntax_valid": true,
      "syntax_error": null,
      "storage_engine": "InnoDB",
      "ok_online_ddl": true,
      "ok_pt_osc": false,
      "ok_online_physical_backup": true,
      "ok_pitr": true
    }
  }
]
```

Required fields:

- `schema_name`: schema selected by the task.
- `ddl_statement`: the DDL statement to execute.
- `analysis_results.schema_name`: schema name identified during analysis.
- `analysis_results.table_name`: target table identified during analysis.

Execution-control fields:

- `analysis_results.syntax_valid`: must be `true`; invalid statements are skipped.
- `analysis_results.syntax_error`: included in task output when syntax validation failed.
- `analysis_results.storage_engine`: used for warning output when not `InnoDB`.
- `analysis_results.ok_online_ddl`: allows execution through native MySQL online DDL.
- `analysis_results.ok_pt_osc`: allows execution through `pt-online-schema-change`.
- `analysis_results.ok_online_physical_backup`: selects `xtrabackup` when a pre-change backup is requested.
- `analysis_results.ok_pitr`: must be `true` when `pre_change_bkp` is requested.
- `pre_change_bkp`: requests a backup before executing the DDL.

## ApplySchemaChanges Flow

`ApplySchemaChanges` performs validation and policy decisions before calling the phase 2 executor:

1. Parse `TaskStruct.Details` as an array. Invalid JSON, an empty array, or missing required fields fails the whole task before any DDL is attempted.
2. Create one `phase2.Executor` using the global MySQL connection (`models.DB`) and the agent logger.
3. Iterate over each schema-change item independently.
4. Skip a statement when `syntax_valid` is `false`.
5. Skip a statement when both `ok_online_ddl` and `ok_pt_osc` are `false`, because the code refuses to run DDL that cannot be executed without blocking the table.
6. Emit a warning when the storage engine is not `InnoDB`.
7. If `pre_change_bkp` is `true`, require `ok_pitr == true`. If point-in-time recovery is unavailable, the statement is skipped.
8. Select a backup method:
   - `none` when `pre_change_bkp` is `false`.
   - `xtrabackup` when `pre_change_bkp` is `true` and `ok_online_physical_backup` is `true`.
   - `mysqldump` when `pre_change_bkp` is `true` and online physical backup is not available.
9. Call `executor.Execute` with the DDL, fully qualified table name, backup method, execution capability flags, config, and debug flag.
10. Mark the task as failed when a statement fails. The loop continues, so later statements can still be attempted.

If every statement is skipped or fails, the task output includes `No schema changes were executed.` and the task returns a failure status.

## ExecuteOptions Passed To Phase 2

`ApplySchemaChanges` builds `phase2.ExecuteOptions` like this:

```go
phase2.ExecuteOptions{
    SQL:          statement,
    TableName:    analysis.SchemaName + "." + analysis.TableName,
    BackupMethod: backupMethod,
    OkPTOSC:      analysis.OKPTOSC,
    OkOnlineDDL:  analysis.OKOnlineDDL,
    Config:       configuration,
    Debug:        configuration.Debug,
}
```

Backups and `pt-online-schema-change` use connection details from `Config`.

## What executor.Execute Does

`phase2.Executor.Execute` owns the operational part of the DDL:

1. Validate that `TableName` is present. The executor does not infer the target table from the DDL; task type 6 passes the analyzed `schema.table` value explicitly.
2. Check datadir filesystem capacity unless `disable_space_checks` is enabled.
3. If a backup method was selected, verify backup directory space and run the backup.
4. Prefer native Online DDL when `OkOnlineDDL` is `true`.
5. Use `pt-online-schema-change` only when Online DDL is not allowed and `OkPTOSC` is `true`.
6. Fail when neither execution method is allowed.

Important behavior: if both `OkOnlineDDL` and `OkPTOSC` are `true`, Online DDL is chosen first. The current implementation does not fall back from a failed Online DDL attempt to `pt-online-schema-change`.

## Native Online DDL Path

When `OkOnlineDDL` is true, the executor:

1. Requires `online_ddl_test_schema`.
2. Creates the test schema if needed.
3. Creates an empty test table using `CREATE TABLE test_table LIKE source_table`.
4. Adds `ALGORITHM=INPLACE` and `LOCK=NONE` to the DDL when those clauses are missing.
5. Rewrites the DDL target to the test table and executes the DDL as a preflight.
6. Drops the test table with `defer`.
7. Sets `SESSION lock_wait_timeout = 20`.
8. Executes the final DDL against the real table.

This path only supports DDL forms that `buildOnlineDDLSQL` can decorate, currently `ALTER TABLE` and common `CREATE INDEX` variants.

## pt-online-schema-change Path

When Online DDL is not allowed and `OkPTOSC` is true, the executor:

1. Builds the connection string from `mysql_host`, `mysql_port`, `mysql_user`, and `mysql_password`.
2. Extracts the `--alter` clause from the original DDL.
3. Runs `pt-online-schema-change --dry-run`.
4. Runs `pt-online-schema-change --execute` only after the dry run succeeds.

The configured `ptosc_path` is used when present; otherwise the binary name defaults to `pt-online-schema-change`.

## Backup Paths

When `pre_change_bkp` selects `mysqldump`, the executor creates:

```text
<backup_dir>/<YYMMDDHHMMSS>_<schema>_<table>.sql
```

The command uses `--single-transaction`, `--quick`, and `--lock-tables=false`.

When `pre_change_bkp` selects `xtrabackup`, the executor creates:

```text
<backup_dir>/<YYMMDDHHMMSS>_xtrabackup_<schema>_<table>/
```

The backup uses `--tables=^schema\.table$`, `--ftwrl-wait-timeout=15`, then runs `xtrabackup --prepare --export`.

## Agent Configuration Parameters

The schema-change executor depends on these `releem.conf` parameters:

| Parameter | Default | Purpose |
| --- | --- | --- |
| `backup_dir` | `/tmp/backups` | Directory used for `mysqldump` files and `xtrabackup` directories. |
| `ptosc_path` | `pt-online-schema-change` | Binary path for Percona Toolkit online schema changes. |
| `mysqldump_path` | `mysqldump` | Binary path for logical table backups. |
| `xtrabackup_path` | `xtrabackup` | Binary path for physical backups. |
| `backup_space_buffer` | `20.0` | Extra free-space percentage required above estimated backup size. |
| `online_ddl_test_schema` | `releem_online_ddl_test` | Scratch schema used for Online DDL preflight tables. |
| `disable_space_checks` | `false` | Disables datadir and backup-directory space checks when set to `true`. |

The executor also uses the existing MySQL connection parameters from the agent config:

- `mysql_host`
- `mysql_port`
- `mysql_user`
- `mysql_password`
- `debug`

When `debug` is enabled, phase 2 logs commands, arguments, preflight SQL, size estimates, and command output. Passwords are masked in debug logs for `mysqldump`, `xtrabackup`, and `pt-online-schema-change` arguments.

## Safety Features

The implementation is intentionally conservative:

- Server-side analysis gates execution through `syntax_valid`, `ok_online_ddl`, `ok_pt_osc`, `ok_online_physical_backup`, and `ok_pitr`.
- Statements that would require blocking table DDL are skipped instead of being run as regular `ALTER TABLE`.
- Pre-change backup is enforced when `pre_change_bkp` is requested.
- Point-in-time recovery must be available before a requested pre-change backup is accepted.
- Backup method selection favors online physical backup when available and falls back to logical backup when not.
- Datadir capacity is checked before any schema change. The filesystem must have more than 10% free space and projected usage after accounting for the table size must stay at or below 90%.
- Backup-directory free space is checked before backup execution. The required space includes `backup_space_buffer`.
- `pt-online-schema-change` always runs `--dry-run` before `--execute`.
- Online DDL is tested on an empty clone of the table before the real table is changed.
- Online DDL execution forces `ALGORITHM=INPLACE` and `LOCK=NONE` unless the DDL already specifies those clauses.
- Online DDL sets `lock_wait_timeout = 20` to avoid waiting indefinitely for metadata locks.
- Debug logging masks passwords while still exposing enough command detail to troubleshoot.

## Operational Notes

- Executor errors are logged and appended to task output as statement-scoped failure messages.
- A task can partially succeed: one statement may fail while later statements still run. Any failure sets the final task status to failed.
- `disable_space_checks` should be used only for controlled testing or when an external capacity check already exists.
