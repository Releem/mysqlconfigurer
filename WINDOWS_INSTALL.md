# Releem Agent Windows Installation

This document describes how to install Releem Agent on Windows systems.

## Prerequisites

1. **MySQL Server** installed and running on Windows
2. **PowerShell 5.1** or later (recommended) or **Command Prompt** with batch support
3. **Administrator privileges** for installation
4. **Releem API Key** (sign up at https://releem.com)

## Installation Methods

### Method 1: PowerShell Script (Recommended)

The PowerShell script provides better error handling and advanced functionality.

#### Basic Installation
```powershell
# Run as Administrator
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
.\install.ps1 -ApiKey "YOUR_API_KEY" -MySQLRootPassword "YOUR_MYSQL_ROOT_PASSWORD"
```

#### Installation with Custom Parameters
```powershell
.\install.ps1 -ApiKey "YOUR_API_KEY" `
              -MySQLRootPassword "YOUR_MYSQL_ROOT_PASSWORD" `
              -MySQLHost "localhost" `
              -MySQLPort "3306" `
              -MemoryLimit 2048 `
              -Region "US"
```

#### Using Environment Variables
```powershell
$env:RELEEM_API_KEY = "YOUR_API_KEY"
$env:RELEEM_MYSQL_ROOT_PASSWORD = "YOUR_MYSQL_ROOT_PASSWORD"
$env:RELEEM_MYSQL_HOST = "localhost"
$env:RELEEM_MYSQL_PORT = "3306"
$env:RELEEM_REGION = "EU"  # or "US"
.\install.ps1
```

### Method 2: Batch File

The batch file provides basic installation functionality for systems where PowerShell is not preferred.

#### Basic Installation
```cmd
REM Run as Administrator
set RELEEM_API_KEY=YOUR_API_KEY
set RELEEM_MYSQL_ROOT_PASSWORD=YOUR_MYSQL_ROOT_PASSWORD
install.bat
```

## Environment Variables

The following environment variables can be used to configure the installation:

| Variable | Description | Example |
|----------|-------------|---------|
| `RELEEM_API_KEY` | Your Releem API key (required) | `rel_1234567890abcdef` |
| `RELEEM_MYSQL_ROOT_PASSWORD` | MySQL root password | `mypassword123` |
| `RELEEM_MYSQL_HOST` | MySQL host address | `localhost`, `127.0.0.1`, `mysql.example.com` |
| `RELEEM_MYSQL_PORT` | MySQL port number | `3306` |
| `RELEEM_MYSQL_LOGIN` | Existing MySQL user (optional) | `releem` |
| `RELEEM_MYSQL_PASSWORD` | Password for existing MySQL user | `userpassword` |
| `RELEEM_MYSQL_MEMORY_LIMIT` | Memory limit in MB | `2048` |
| `RELEEM_HOSTNAME` | Custom hostname | `myserver` |
| `RELEEM_REGION` | Releem region (US/EU) | `EU` |
| `RELEEM_ENV` | Environment name | `production` |
| `RELEEM_DEBUG` | Enable debug mode | `true` |
| `RELEEM_QUERY_OPTIMIZATION` | Enable query optimization | `true` |
| `RELEEM_CRON_ENABLE` | Auto-create scheduled task | `1` |
| `RELEEM_AGENT_DISABLE` | Skip agent startup | `1` |

## Command Line Options (PowerShell)

| Option | Description |
|--------|-------------|
| `-Update` | Update existing installation |
| `-Uninstall` | Remove Releem Agent |
| `-EnableQueryOptimization` | Enable query optimization feature |
| `-SkipCron` | Don't create scheduled task |
| `-DisableAgent` | Don't start agent after installation |

## Installation Process

The installation script performs the following steps:

1. **Validates prerequisites** - Checks for MySQL installation and API key
2. **Downloads components** - Fetches the latest Releem Agent and configuration scripts
3. **Configures MySQL user** - Creates a dedicated `releem` user with required privileges
4. **Sets up configuration** - Creates configuration files in `C:\Program Files\Releem\`
5. **Installs Windows service** - Registers Releem Agent as a Windows service
6. **Creates scheduled task** - Sets up automatic configuration updates (daily at midnight)
7. **Starts monitoring** - Begins collecting MySQL metrics

## File Locations

- **Installation directory**: `C:\Program Files\Releem\`
- **Configuration file**: `C:\Program Files\Releem\releem.conf`
- **Log files**: `%TEMP%\releem-install.log`
- **MySQL configuration**: `C:\Program Files\Releem\conf\`

## MySQL Privileges

The installation creates a MySQL user named `releem` with the following privileges:

```sql
GRANT PROCESS ON *.* TO 'releem'@'host';
GRANT REPLICATION CLIENT ON *.* TO 'releem'@'host';
GRANT SHOW VIEW ON *.* TO 'releem'@'host';
GRANT SELECT ON mysql.* TO 'releem'@'host';
GRANT SELECT ON performance_schema.* TO 'releem'@'host';
GRANT SYSTEM_VARIABLES_ADMIN ON *.* TO 'releem'@'host'; -- or SUPER for older versions
```

## Service Management

After installation, you can manage the Releem Agent service using:

```cmd
# Start service
net start "Releem Agent"

# Stop service
net stop "Releem Agent"

# Check service status
sc query "Releem Agent"
```

Or using the agent executable:

```cmd
cd "C:\Program Files\Releem"
releem-agent.exe start
releem-agent.exe stop
releem-agent.exe status
```

## Scheduled Task

The installation creates a scheduled task named "Releem Agent Config Update" that runs daily at midnight to check for configuration updates. You can manage it using:

```powershell
# View task
Get-ScheduledTask -TaskName "Releem Agent Config Update"

# Disable task
Disable-ScheduledTask -TaskName "Releem Agent Config Update"

# Enable task
Enable-ScheduledTask -TaskName "Releem Agent Config Update"

# Remove task
Unregister-ScheduledTask -TaskName "Releem Agent Config Update" -Confirm:$false
```

## Updating

To update Releem Agent to the latest version:

```powershell
# PowerShell
.\install.ps1 -Update

# Batch
install.bat -u
```

## Uninstalling

To completely remove Releem Agent:

```powershell
# PowerShell
.\install.ps1 -Uninstall

# Batch
install.bat uninstall
```

## Troubleshooting

### Common Issues

1. **MySQL connection failed**
   - Verify MySQL is running: `Get-Service -Name MySQL*`
   - Check credentials and host/port settings
   - Ensure MySQL allows connections from the specified host

2. **Permission denied errors**
   - Run the installation script as Administrator
   - Check that the user has rights to create services and scheduled tasks

3. **Download failures**
   - Check internet connectivity
   - Verify Windows firewall/antivirus isn't blocking downloads
   - Try running with `-Verbose` flag for more details

4. **Service won't start**
   - Check Windows Event Log (Application and System logs)
   - Verify configuration file syntax in `C:\Program Files\Releem\releem.conf`
   - Ensure MySQL user has required privileges

### Log Files

Check the installation log for detailed error information:
```powershell
Get-Content "$env:TEMP\releem-install.log" -Tail 50
```

### Manual Service Installation

If automatic service installation fails:

```cmd
cd "C:\Program Files\Releem"
releem-agent.exe install
releem-agent.exe start
```

## Support

For additional support:
- Email: hello@releem.com
- Documentation: https://docs.releem.com
- Dashboard: https://app.releem.com

Include the installation log file when reporting issues.
