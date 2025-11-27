# PostgreSQL Support for Releem Agent

This document describes the PostgreSQL metrics collection support added to the Releem Agent.

## Overview

The Releem Agent now supports collecting metrics from PostgreSQL databases in addition to MySQL. The agent automatically detects the database type based on the configuration parameters and initializes the appropriate metrics gatherers.

## Database Type Detection

The agent determines the database type based on the presence of configuration parameters:

- **PostgreSQL**: If `pg_user` and `pg_password` are configured
- **MySQL**: If `mysql_user` and `mysql_password` are configured (default for backward compatibility)

## Configuration

### PostgreSQL Configuration Parameters

Add the following parameters to your `releem.conf` file for PostgreSQL support:

```hcl
# PostgreSQL Connection Parameters
pg_user="releem"                    # PostgreSQL username
pg_password="releem_password"       # PostgreSQL password  
pg_host="127.0.0.1"                # PostgreSQL host (default: 127.0.0.1)
pg_port="5432"                     # PostgreSQL port (default: 5432)
pg_database="postgres"             # PostgreSQL database (default: postgres)
pg_ssl_mode="disable"              # SSL mode: disable, require, verify-ca, verify-full (default: disable)
```

### Example Configuration

See `releem-postgresql.conf.example` for a complete PostgreSQL configuration example.

## Prerequisites

### PostgreSQL Extensions

For optimal query performance monitoring, install the `pg_stat_statements` extension:

```sql
-- Connect as superuser
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

-- Add to postgresql.conf
shared_preload_libraries = 'pg_stat_statements'

-- Add to pg_hba.conf 
host    all             releem          0.0.0.0/0               md5


-- Restart PostgreSQL service
```

### Database User Permissions

Create a monitoring user with appropriate permissions:

```sql
-- Create monitoring user
CREATE USER releem WITH PASSWORD 'secure_password';

-- Grant necessary permissions
GRANT CONNECT ON DATABASE postgres TO releem;
GRANT USAGE ON SCHEMA public TO releem;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO releem;
GRANT SELECT ON ALL TABLES IN SCHEMA information_schema TO releem;
GRANT SELECT ON ALL TABLES IN SCHEMA pg_catalog TO releem;

-- For pg_stat_statements access
GRANT EXECUTE ON FUNCTION pg_stat_statements_reset() TO releem;
```

## Metrics Collected

### PostgreSQL Base Metrics (`pgMetricsBase.go`)

- **Database Statistics**: From `pg_stat_database`
  - Transaction counts (commits, rollbacks)
  - Block I/O statistics (reads, hits)
  - Tuple operations (returned, fetched, inserted, updated, deleted)
  - Conflicts, temp files, deadlocks
  - Checksum failures, I/O timing

- **Background Writer Statistics**: From `pg_stat_bgwriter`
  - Checkpoint statistics
  - Buffer allocation and cleaning
  - Backend buffer operations

- **Connection Statistics**: From `pg_stat_activity`
  - Total, active, and idle connections

### PostgreSQL Configuration (`pgConf.go`)

- **Database Settings**: From `pg_settings`
  - All PostgreSQL configuration parameters
  - Parameter metadata (unit, category, context, type, source)
  - Min/max values, enumerated values
  - Boot and reset values
  - Pending restart status

### PostgreSQL Database Info (`pgInfo.go`)

- **Database Information**:
  - PostgreSQL version
  - List of user databases (excluding templates)
  - Total table count
  - Table type statistics (BASE TABLE, VIEW, MATERIALIZED VIEW)
  - Table size information

### PostgreSQL Query Performance (`pgCollectQueries.go`)

- **Query Statistics**: From `pg_stat_statements`
  - Query execution statistics (calls, timing)
  - Query text and identifiers
  - Row statistics
  - Database-specific query performance

- **Schema Information**: From `information_schema`
  - Table metadata
  - Column definitions
  - Index information (when available)

## Implementation Details

### File Structure

```
metrics/
├── pgMetricsBase.go      # PostgreSQL system metrics
├── pgConf.go            # PostgreSQL configuration metrics
├── pgInfo.go            # PostgreSQL database information
└── pgCollectQueries.go  # PostgreSQL query performance metrics
```

### Database Connection

The connection logic in `utils/utils.go` has been enhanced to support both database types:

- `ConnectionDatabase()`: Main entry point with database type detection
- `ConnectionMySQL()`: MySQL-specific connection logic
- `ConnectionPostgreSQL()`: PostgreSQL-specific connection logic

### Query Adaptations

| MySQL Source | PostgreSQL Equivalent |
|--------------|----------------------|
| `SHOW STATUS` | `pg_stat_database`, `pg_stat_bgwriter` |
| `SHOW VARIABLES` | `pg_settings` |
| `INFORMATION_SCHEMA.tables` | `information_schema.tables` (PostgreSQL format) |
| `performance_schema.events_statements_summary_by_digest` | `pg_stat_statements` |

## Backward Compatibility

- Existing MySQL configurations continue to work unchanged
- Default database type is MySQL for backward compatibility
- All existing MySQL functionality is preserved

## Troubleshooting

### Common Issues

1. **pg_stat_statements not available**
   - Install and configure the extension as described above
   - The agent will skip query collection if the extension is not available

2. **Permission denied errors**
   - Ensure the monitoring user has appropriate permissions
   - Check database connection parameters

3. **Connection failures**
   - Verify PostgreSQL is running and accepting connections
   - Check host, port, and SSL configuration
   - Ensure the database exists and is accessible

### Logging

Enable debug logging to troubleshoot connection and metrics collection issues:

```hcl
debug=true
```

## Migration from MySQL

To migrate an existing Releem Agent installation from MySQL to PostgreSQL:

1. Stop the Releem Agent service
2. Update `releem.conf` with PostgreSQL parameters
3. Remove or comment out MySQL parameters
4. Ensure PostgreSQL prerequisites are met
5. Start the Releem Agent service

The agent will automatically detect the database type change and use PostgreSQL gatherers.
