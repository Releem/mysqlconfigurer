#Requires -Version 5.1

<#
.SYNOPSIS
    Releem MySQL Configurer - Windows PowerShell Edition

.DESCRIPTION
    Manages MySQL configuration recommendations from Releem Platform.
    Port of mysqlconfigurer.sh v1.22.1 for Windows environments.

.PARAMETER ApiKey
    Releem API Key (overrides value from releem.conf)

.PARAMETER MemoryLimit
    MySQL Memory Limit in MB (overrides value from releem.conf)

.PARAMETER ApplyManual
    Apply recommended configuration interactively

.PARAMETER ApplyConfig
    Apply configuration mode: "auto" (create API job) or "automatic" (direct apply)

.PARAMETER Rollback
    Rollback to previous MySQL configuration

.PARAMETER EnablePerformanceSchema
    Enable Performance Schema and Slow Query Log

.PARAMETER UpdateAgent
    Check for and install agent updates

.EXAMPLE
    .\mysqlconfigurer.ps1 -ApplyManual
    Apply recommended configuration with user confirmation

.EXAMPLE
    .\mysqlconfigurer.ps1 -EnablePerformanceSchema
    Enable Performance Schema for query optimization

.EXAMPLE
    .\mysqlconfigurer.ps1 -Rollback
    Rollback to previous configuration

.EXAMPLE
    .\mysqlconfigurer.ps1 -UpdateAgent
    Check for and install agent updates

.NOTES
    Version: 1.22.0
    Author: Releem, Inc
    Copyright: (C) Releem, Inc 2022-2025
#>

[CmdletBinding()]
Param(
    [Parameter(Mandatory=$false)]
    [Alias('k')]
    [string]$ApiKey,

    [Parameter(Mandatory=$false)]
    [Alias('m')]
    [int]$MemoryLimit,

    [Parameter(Mandatory=$false)]
    [Alias('a')]
    [switch]$ApplyManual,

    [Parameter(Mandatory=$false)]
    [Alias('s')]
    [ValidateSet('auto', 'automatic')]
    [string]$ApplyConfig,

    [Parameter(Mandatory=$false)]
    [Alias('r')]
    [switch]$Rollback,

    [Parameter(Mandatory=$false)]
    [Alias('p')]
    [switch]$EnablePerformanceSchema,

    [Parameter(Mandatory=$false)]
    [Alias('u')]
    [switch]$UpdateAgent
)

#region Script Variables

$script:VERSION = "1.22.0"
$script:RELEEM_PATH = "C:\ProgramData\Releem"
$script:RELEEM_INSTALL_PATH = "C:\Program Files\Releem"
$script:RELEEM_CONF_FILE = Join-Path $RELEEM_PATH "releem.conf"
$script:MYSQLCONFIGURER_PATH = Join-Path $RELEEM_PATH "conf"
$script:MYSQLCONFIGURER_FILE_NAME = "z_aiops_mysql.cnf"
$script:INITIAL_MYSQLCONFIGURER_FILE_NAME = "initial_config_mysql.cnf"
$script:RELEEM_MYSQL_VERSION = Join-Path $MYSQLCONFIGURER_PATH "mysql_version"
$script:MYSQLCONFIGURER_CONFIGFILE = Join-Path $MYSQLCONFIGURER_PATH $MYSQLCONFIGURER_FILE_NAME
$script:LOGFILE = Join-Path $RELEEM_PATH "logs\releem-mysqlconfigurer.log"
$script:RELEEM_AGENT_EXE = Join-Path $RELEEM_INSTALL_PATH "releem-agent.exe"

# Configuration variables (loaded from releem.conf)
$script:RELEEM_API_KEY = $null
$script:MYSQL_MEMORY_LIMIT = 0
$script:MYSQL_CONFIG_DIR = $null
$script:MYSQL_RESTART_SERVICE = $null
$script:MYSQL_LOGIN = $null
$script:MYSQL_PASSWORD = $null
$script:MYSQL_HOST = "127.0.0.1"
$script:MYSQL_PORT = "3306"
$script:RELEEM_QUERY_OPTIMIZATION = $false
$script:RELEEM_REGION = $null
$script:RELEEM_RESTART_SERVICE = $null
$script:API_DOMAIN = "api.releem.com"

#endregion

#region Helper Functions

Function Write-ColorOutput {
    <#
    .SYNOPSIS
        Write colored output to console and log file
    #>
    Param(
        [Parameter(Mandatory=$true)]
        [string]$Message,

        [Parameter(Mandatory=$false)]
        [ValidateSet('White', 'Red', 'Green', 'Yellow', 'Cyan', 'Gray')]
        [string]$Color = 'White'
    )

    $timestamp = Get-Date -Format "yyyyMMdd-HH:mm:ss"
    $logMessage = "[$timestamp] $Message"

    # Ensure log directory exists
    $logDir = Split-Path $script:LOGFILE
    if (-not (Test-Path $logDir)) {
        New-Item -ItemType Directory -Path $logDir -Force | Out-Null
    }

    # Write to log file
    Add-Content -Path $script:LOGFILE -Value $logMessage -ErrorAction SilentlyContinue

    # Write to console with color
    Write-Host $Message -ForegroundColor $Color
}

Function Send-LogToAPI {
    <#
    .SYNOPSIS
        Upload log file to Releem API on exit
    #>
    try {
        if ($script:RELEEM_API_KEY -and (Test-Path $script:LOGFILE)) {
            $logContent = Get-Content -Path $script:LOGFILE -Raw -ErrorAction SilentlyContinue

            if ($logContent) {
                $headers = @{
                    "x-releem-api-key" = $script:RELEEM_API_KEY
                    "Content-Type" = "application/json"
                }

                $uri = "https://$script:API_DOMAIN/v2/events/configurer_log"
                Invoke-RestMethod -Uri $uri -Method Post -Body $logContent -Headers $headers -ErrorAction SilentlyContinue | Out-Null
            }
        }
    }
    catch {
        # Silently fail - don't block script exit
    }
}

Function Load-Configuration {
    <#
    .SYNOPSIS
        Load configuration from releem.conf file
    #>
    if (-not (Test-Path $script:RELEEM_CONF_FILE)) {
        Write-ColorOutput "ERROR: Configuration file not found: $script:RELEEM_CONF_FILE" "Red"
        Write-ColorOutput "Please ensure Releem Agent is installed properly" "Yellow"
        exit 1
    }

    try {
        Get-Content $script:RELEEM_CONF_FILE | ForEach-Object {
            # Skip comments and empty lines
            if ($_ -match '^\s*#' -or $_ -match '^\s*$') {
                return
            }

            # Parse key=value pairs
            if ($_ -match '^\s*([^=]+)\s*=\s*"?([^"]*)"?\s*$') {
                $key = $matches[1].Trim()
                $value = $matches[2].Trim().Trim('"')

                switch ($key) {
                    "apikey" { $script:RELEEM_API_KEY = $value }
                    "memory_limit" {
                        if ($value) { $script:MYSQL_MEMORY_LIMIT = [int]$value }
                    }
                    "mysql_cnf_dir" { $script:MYSQL_CONFIG_DIR = $value }
                    "mysql_restart_service" { $script:MYSQL_RESTART_SERVICE = $value }
                    "mysql_user" { $script:MYSQL_LOGIN = $value }
                    "mysql_password" { $script:MYSQL_PASSWORD = $value }
                    "mysql_host" { $script:MYSQL_HOST = $value }
                    "mysql_port" { $script:MYSQL_PORT = $value }
                    "query_optimization" { $script:RELEEM_QUERY_OPTIMIZATION = ($value -eq "true") }
                    "releem_region" { $script:RELEEM_REGION = $value }
                }
            }
        }

        # Set API domain based on region
        if ($script:RELEEM_REGION -eq "EU") {
            $script:API_DOMAIN = "api.eu.releem.com"
        }

        # Override with command line parameters
        if ($ApiKey) { $script:RELEEM_API_KEY = $ApiKey }
        if ($MemoryLimit) { $script:MYSQL_MEMORY_LIMIT = $MemoryLimit }

        # Ensure required directories exist
        if (-not (Test-Path $script:MYSQLCONFIGURER_PATH)) {
            New-Item -ItemType Directory -Path $script:MYSQLCONFIGURER_PATH -Force | Out-Null
        }
    }
    catch {
        Write-ColorOutput "ERROR loading configuration: $_" "Red"
        exit 1
    }
}

Function Get-MySQLVersion {
    <#
    .SYNOPSIS
        Get MySQL version from version file
    #>
    try {
        # Try mysql_version file
        if (Test-Path $script:RELEEM_MYSQL_VERSION) {
            $version = Get-Content $script:RELEEM_MYSQL_VERSION -Raw
            return $version.Trim()
        }

        return $null
    }
    catch {
        return $null
    }
}

Function Test-MySQLVersion {
    <#
    .SYNOPSIS
        Check if MySQL version meets minimum requirement (>= 5.6.8)
    #>
    $version = Get-MySQLVersion

    if (-not $version) {
        Write-ColorOutput " " "White"
        Write-ColorOutput " * Please try again later or run Releem Agent manually:" "White"
        Write-ColorOutput "   $script:RELEEM_AGENT_EXE -f" "Green"
        Write-ColorOutput " " "White"
        return $false
    }

    try {
        $requiredVersion = [version]"5.6.8"
        $currentVersion = [version]$version

        return $currentVersion -ge $requiredVersion
    }
    catch {
        return $false
    }
}

Function Invoke-MySQLCommand {
    <#
    .SYNOPSIS
        Execute MySQL command and return output
    #>
    Param(
        [Parameter(Mandatory=$true)]
        [string]$Query
    )

    try {
        $mysqlExe = "mysql.exe"
        $args = @(
            "--host=$script:MYSQL_HOST",
            "--port=$script:MYSQL_PORT",
            "--user=$script:MYSQL_LOGIN",
            "--password=$script:MYSQL_PASSWORD",
            "-BNe",
            "`"$Query`""
        )

        $result = & $mysqlExe @args 2>&1

        if ($LASTEXITCODE -eq 0) {
            return $result
        }
        else {
            return $null
        }
    }
    catch {
        Write-ColorOutput "ERROR executing MySQL query: $_" "Red"
        return $null
    }
}

Function Wait-ServiceRestart {
    <#
    .SYNOPSIS
        Wait for MySQL service to restart with progress indicator
    #>
    Param(
        [Parameter(Mandatory=$true)]
        [int]$ProcessId
    )

    Write-Host " "
    $spinner = @('-', '\', '|', '/')
    $spinnerIndex = 0
    $timeout = 1200  # 20 minutes
    $elapsed = 0

    Write-Host " Waiting for MySQL service to start 1200 seconds " -NoNewline -ForegroundColor White

    while ($elapsed -lt $timeout) {
        # Check if process still exists
        try {
            $process = Get-Process -Id $ProcessId -ErrorAction SilentlyContinue

            if (-not $process) {
                # Process has exited - check exit code
                Write-Host ""
                return 0
            }
        }
        catch {
            # Process doesn't exist
            Write-Host ""
            return 0
        }

        # Show spinner
        Write-Host "`b$($spinner[$spinnerIndex % 4])" -NoNewline
        $spinnerIndex++

        Start-Sleep -Seconds 1
        $elapsed++
    }

    Write-Host ""
    return 6  # Timeout
}

#endregion

#region Core Functions

Function Update-Agent {
    <#
    .SYNOPSIS
        Check for and install agent updates
    #>
    Write-ColorOutput "Checking for agent updates..." "Cyan"

    # Start agent service
    try {
        & $script:RELEEM_AGENT_EXE start 2>&1 | Out-Null
    }
    catch {
        # Ignore errors
    }

    # Get current version from S3
    try {
        $newVersion = (Invoke-RestMethod -Uri "https://releem.s3.amazonaws.com/v2/current_version_agent" -TimeoutSec 10).Trim()

        if ($newVersion -and $newVersion -ne $script:VERSION) {
            # Check if new version is actually newer
            try {
                $currentVer = [version]$script:VERSION
                $newVer = [version]$newVersion

                if ($newVer -le $currentVer) {
                    Write-ColorOutput "Current version ($script:VERSION) is up to date or newer than available version ($newVersion)" "Green"
                    return
                }
            }
            catch {
                # If version parsing fails, proceed with update
            }

            Write-Host " "
            Write-Host " * Updating script " -NoNewline -ForegroundColor White
            Write-Host $script:VERSION -NoNewline -ForegroundColor Red
            Write-Host " -> " -NoNewline -ForegroundColor White
            Write-Host $newVersion -ForegroundColor Green

            # Stop the agent
            Write-ColorOutput "Stopping Releem Agent..." "Yellow"
            & $script:RELEEM_AGENT_EXE stop 2>&1 | Out-Null
            Start-Sleep -Seconds 2

            # Download new agent executable
            $tempExe = "$script:RELEEM_AGENT_EXE.new"
            Write-ColorOutput "Downloading new agent version..." "Cyan"
            Invoke-WebRequest -Uri "https://releem.s3.amazonaws.com/v2/releem-agent-windows.exe" -OutFile $tempExe -ErrorAction Stop

            # Replace the old executable
            Write-ColorOutput "Installing new version..." "Cyan"
            Move-Item -Path $tempExe -Destination $script:RELEEM_AGENT_EXE -Force

            # Download updated mysqlconfigurer.ps1 if available
            try {
                $tempScript = "$PSCommandPath.new"
                Invoke-WebRequest -Uri "https://releem.s3.amazonaws.com/v2/mysqlconfigurer.ps1" -OutFile $tempScript -ErrorAction Stop
                Move-Item -Path $tempScript -Destination $PSCommandPath -Force
                Write-ColorOutput "Updated mysqlconfigurer.ps1" "Green"
            }
            catch {
                # Script update is optional
                Write-ColorOutput "No script update available" "Gray"
            }

            # Start the agent
            Write-ColorOutput "Starting Releem Agent..." "Green"
            & $script:RELEEM_AGENT_EXE start 2>&1 | Out-Null

            # Run agent once
            & $script:RELEEM_AGENT_EXE -f 2>&1 | Out-Null

            # Send event
            & $script:RELEEM_AGENT_EXE --event=agent_updated 2>&1 | Out-Null

            Write-Host " "
            Write-ColorOutput "Releem Agent updated successfully." "Green"
            Write-Host " "
        }
        else {
            Write-ColorOutput "Agent is up to date ($script:VERSION)" "Green"
        }
    }
    catch {
        Write-ColorOutput "ERROR checking for updates: $_" "Red"
    }
}

Function Enable-PerformanceSchema {
    <#
    .SYNOPSIS
        Enable Performance Schema and Slow Query Log
    #>
    $FLAG_CONFIGURE = 1

    # Check current performance_schema status
    $status_ps = Invoke-MySQLCommand "show global variables like 'performance_schema'"
    if ($status_ps -and $status_ps -match "\s+ON\s*$") {
        # Already ON
    }
    else {
        $FLAG_CONFIGURE = 0
    }

    # Check slow_query_log status
    $status_slowlog = Invoke-MySQLCommand "show global variables like 'slow_query_log'"
    if ($status_slowlog -and $status_slowlog -match "\s+ON\s*$") {
        # Already ON
    }
    else {
        $FLAG_CONFIGURE = 0
    }

    # Verify config directory exists
    if (-not $script:MYSQL_CONFIG_DIR -or -not (Test-Path $script:MYSQL_CONFIG_DIR)) {
        Write-ColorOutput " " "Red"
        Write-ColorOutput " MySQL configuration directory was not found." "Red"
        Write-ColorOutput " Try to reinstall Releem Agent." "Red"
        exit 3
    }

    Write-ColorOutput " " "White"
    Write-ColorOutput " * Enabling and configuring Performance schema and SlowLog to collect metrics and queries." "White"
    Write-ColorOutput " " "White"

    # Create collect_metrics.cnf
    $collectMetricsFile = Join-Path $script:MYSQL_CONFIG_DIR "collect_metrics.cnf"
    $configContent = @"
### This configuration was recommended by Releem. https://releem.com
[mysqld]
performance_schema = 1
slow_query_log = 1
"@

    # Check query optimization settings
    if ($script:RELEEM_QUERY_OPTIMIZATION) {
        if (-not (Test-MySQLVersion)) {
            Write-ColorOutput " " "Red"
            Write-ColorOutput " * MySQL version is lower than 5.6.7. Query optimization is not supported. Please reinstall the agent with query optimization disabled." "Red"
            Write-ColorOutput " " "Red"
        }
        else {
            $events_statements_current = Invoke-MySQLCommand "SELECT ENABLED FROM performance_schema.setup_consumers WHERE NAME = 'events_statements_current'"
            $events_statements_history = Invoke-MySQLCommand "SELECT ENABLED FROM performance_schema.setup_consumers WHERE NAME = 'events_statements_history'"

            if ($events_statements_current -ne "YES") { $FLAG_CONFIGURE = 0 }
            if ($events_statements_history -ne "YES") { $FLAG_CONFIGURE = 0 }

            $configContent += "`nperformance-schema-consumer-events-statements-history = ON"
            $configContent += "`nperformance-schema-consumer-events-statements-current = ON"
        }
    }

    # Write configuration file
    Set-Content -Path $collectMetricsFile -Value $configContent -Force

    if ($FLAG_CONFIGURE -eq 1) {
        Write-ColorOutput " " "White"
        Write-ColorOutput " * Performance schema and SlowLog are enabled and configured to collect metrics and queries." "White"
        exit 0
    }

    # Need to restart
    Write-ColorOutput " To apply changes to the MySQL configuration, you need to restart the service" "White"
    Write-ColorOutput " " "White"

    $FLAG_RESTART_SERVICE = 1
    if (-not $script:RELEEM_RESTART_SERVICE) {
        $response = Read-Host " Restart MySQL service? (Y/N)"
        if ($response -notmatch '^[Yy]') {
            Write-ColorOutput " Confirmation to restart the service has not been received." "Red"
            $FLAG_RESTART_SERVICE = 0
        }
    }
    elseif ($script:RELEEM_RESTART_SERVICE -eq "0") {
        $FLAG_RESTART_SERVICE = 0
    }

    if ($FLAG_RESTART_SERVICE -eq 0) {
        Write-ColorOutput " " "Red"
        Write-ColorOutput " For applying change in configuration MySQL need to restart service." "Red"
        Write-ColorOutput " Run the command `"$PSCommandPath -p`" when it is possible to restart the service." "Red"
        exit 0
    }

    # Restart MySQL service
    Write-ColorOutput " Restarting MySQL service with command '$script:MYSQL_RESTART_SERVICE'." "White"

    try {
        $job = Start-Job -ScriptBlock {
            param($cmd)
            Invoke-Expression $cmd
        } -ArgumentList $script:MYSQL_RESTART_SERVICE

        $RESTART_CODE = Wait-ServiceRestart -ProcessId $job.Id
        Remove-Job -Job $job -Force -ErrorAction SilentlyContinue

        if ($RESTART_CODE -eq 0) {
            Write-ColorOutput " " "Green"
            Write-ColorOutput " The MySQL service restarted successfully!" "Green"
            Write-ColorOutput " Performance schema and SlowLog are enabled and configured to collect metrics and queries." "Green"
        }
        elseif ($RESTART_CODE -eq 6) {
            Write-ColorOutput " " "Red"
            Write-ColorOutput " The MySQL service failed to restart in 1200 seconds. Check the MySQL error log." "Red"
        }
        elseif ($RESTART_CODE -eq 7) {
            $timestamp = Get-Date -Format "yyyyMMdd-HH:mm:ss"
            Write-ColorOutput " " "Red"
            Write-ColorOutput " $timestamp The MySQL service failed to restart with error. Check the MySQL error log." "Red"
        }

        Write-ColorOutput " Sending notification to Releem Platform." "Green"
        & $script:RELEEM_AGENT_EXE -f 2>&1 | Out-Null

        exit $RESTART_CODE
    }
    catch {
        Write-ColorOutput "ERROR restarting MySQL: $_" "Red"
        exit 7
    }
}

Function Restore-Configuration {
    <#
    .SYNOPSIS
        Rollback MySQL configuration to previous state
    #>
    Write-ColorOutput " " "Red"
    Write-ColorOutput " * Rolling back MySQL configuration." "Red"

    # Check MySQL version
    if (-not (Test-MySQLVersion)) {
        Write-ColorOutput " " "Red"
        Write-ColorOutput " * MySQL version is lower than 5.6.7. Check the documentation https://github.com/Releem/mysqlconfigurer#how-to-apply-the-recommended-configuration for applying the configuration." "Red"
        Write-ColorOutput " " "Red"
        exit 2
    }

    # Verify config directory
    if (-not $script:MYSQL_CONFIG_DIR -or -not (Test-Path $script:MYSQL_CONFIG_DIR)) {
        Write-ColorOutput " " "White"
        Write-ColorOutput " * MySQL configuration directory was not found." "White"
        Write-ColorOutput " * Try to reinstall Releem Agent, and set the my.cnf location." "White"
        exit 3
    }

    # Prompt for restart
    $FLAG_RESTART_SERVICE = 1
    if (-not $script:RELEEM_RESTART_SERVICE) {
        $response = Read-Host "Restart MySQL service? (Y/N)"
        if ($response -notmatch '^[Yy]') {
            Write-ColorOutput " " "White"
            Write-ColorOutput " * Confirmation to restart the service has not been received. Releem recommended configuration has not been rolled back." "White"
            $FLAG_RESTART_SERVICE = 0
        }
    }
    elseif ($script:RELEEM_RESTART_SERVICE -eq "0") {
        $FLAG_RESTART_SERVICE = 0
    }

    if ($FLAG_RESTART_SERVICE -eq 0) {
        exit 5
    }

    # Delete current config
    $currentConfig = Join-Path $script:MYSQL_CONFIG_DIR $script:MYSQLCONFIGURER_FILE_NAME
    Write-ColorOutput " " "Red"
    Write-ColorOutput " * Deleting the configuration file." "Red"
    if (Test-Path $currentConfig) {
        Remove-Item $currentConfig -Force -ErrorAction SilentlyContinue
    }

    # Restore backup if exists
    $backupFile = "$script:MYSQLCONFIGURER_CONFIGFILE.bkp"
    if (Test-Path $backupFile) {
        Write-ColorOutput " " "Red"
        Write-ColorOutput " * Restoring the backup copy of the configuration file $backupFile." "Red"
        Copy-Item $backupFile $currentConfig -Force
    }

    # Check restart command
    if (-not $script:MYSQL_RESTART_SERVICE) {
        Write-ColorOutput " " "White"
        Write-ColorOutput " * The command to restart the MySQL service was not found. Try to reinstall Releem Agent." "White"
        exit 4
    }

    # Restart MySQL
    Write-ColorOutput " " "Red"
    Write-ColorOutput " * Restarting MySQL with command '$script:MYSQL_RESTART_SERVICE'." "Red"

    try {
        $job = Start-Job -ScriptBlock {
            param($cmd)
            Invoke-Expression $cmd
        } -ArgumentList $script:MYSQL_RESTART_SERVICE

        $RESTART_CODE = Wait-ServiceRestart -ProcessId $job.Id
        Remove-Job -Job $job -Force -ErrorAction SilentlyContinue

        if ($RESTART_CODE -eq 0) {
            $timestamp = Get-Date -Format "yyyyMMdd-HH:mm:ss"
            Write-ColorOutput " " "Green"
            Write-ColorOutput " $timestamp The MySQL service restarted successfully!" "Green"
            if (Test-Path $backupFile) {
                Remove-Item $backupFile -Force -ErrorAction SilentlyContinue
            }
        }
        elseif ($RESTART_CODE -eq 6) {
            $timestamp = Get-Date -Format "yyyyMMdd-HH:mm:ss"
            Write-ColorOutput " " "Red"
            Write-ColorOutput " $timestamp The MySQL service failed to restart in 1200 seconds. Check the MySQL error log." "Red"
        }
        elseif ($RESTART_CODE -eq 7) {
            $timestamp = Get-Date -Format "yyyyMMdd-HH:mm:ss"
            Write-ColorOutput " " "Red"
            Write-ColorOutput " $timestamp The MySQL service failed to restart. Check the MySQL error log." "Red"
        }

        & $script:RELEEM_AGENT_EXE --event=config_rollback 2>&1 | Out-Null
        exit $RESTART_CODE
    }
    catch {
        Write-ColorOutput "ERROR during rollback: $_" "Red"
        exit 7
    }
}

Function Apply-ConfigurationManual {
    <#
    .SYNOPSIS
        Apply recommended MySQL configuration interactively
    #>
    # Check if config file exists
    if (-not (Test-Path $script:MYSQLCONFIGURER_CONFIGFILE)) {
        Write-ColorOutput " " "White"
        Write-ColorOutput " * Recommended MySQL configuration was not found." "White"
        Write-ColorOutput " * Please apply recommended configuration later or run Releem Agent manually:" "White"
        Write-ColorOutput "   $script:RELEEM_AGENT_EXE -f" "Green"
        Write-ColorOutput " " "White"
        exit 1
    }

    # Check MySQL version
    if (-not (Test-MySQLVersion)) {
        Write-ColorOutput " " "Red"
        Write-ColorOutput " * MySQL version is lower than 5.6.7. Check the documentation https://github.com/Releem/mysqlconfigurer#how-to-apply-the-recommended-configuration for applying the configuration." "Red"
        Write-ColorOutput " " "Red"
        exit 2
    }

    # Verify config directory
    if (-not $script:MYSQL_CONFIG_DIR -or -not (Test-Path $script:MYSQL_CONFIG_DIR)) {
        Write-ColorOutput " " "White"
        Write-ColorOutput " * MySQL configuration directory was not found." "White"
        Write-ColorOutput " * Try to reinstall Releem Agent, and please set the my.cnf location." "White"
        exit 3
    }

    # Apply configuration
    $timestamp = Get-Date -Format "yyyyMMdd-HH:mm:ss"
    Write-ColorOutput " " "White"
    Write-ColorOutput " $timestamp Applying the recommended MySQL configuration." "White"
    Write-ColorOutput " " "White"
    Write-ColorOutput " $timestamp Getting the latest up-to-date configuration." "White"

    & $script:RELEEM_AGENT_EXE -c 2>&1 | Out-Null

    # Compare configurations
    $currentConfig = Join-Path $script:MYSQL_CONFIG_DIR $script:MYSQLCONFIGURER_FILE_NAME
    if (Test-Path $currentConfig) {
        $diff = Compare-Object -ReferenceObject (Get-Content $currentConfig) -DifferenceObject (Get-Content $script:MYSQLCONFIGURER_CONFIGFILE) -ErrorAction SilentlyContinue

        if (-not $diff) {
            $timestamp = Get-Date -Format "yyyyMMdd-HH:mm:ss"
            Write-ColorOutput " " "Green"
            Write-ColorOutput " $timestamp The new configuration is identical to the current configuration. No restart is required!" "Green"
            exit 0
        }
    }

    # Prompt for restart
    $FLAG_RESTART_SERVICE = 1
    if (-not $script:RELEEM_RESTART_SERVICE) {
        $response = Read-Host "Restart MySQL service? (Y/N)"
        if ($response -notmatch '^[Yy]') {
            $timestamp = Get-Date -Format "yyyyMMdd-HH:mm:ss"
            Write-ColorOutput " " "White"
            Write-ColorOutput " $timestamp Confirmation to restart the service has not been received. Releem recommended configuration has not been applied." "White"
            $FLAG_RESTART_SERVICE = 0
        }
    }
    elseif ($script:RELEEM_RESTART_SERVICE -eq "0") {
        $FLAG_RESTART_SERVICE = 0
    }

    if ($FLAG_RESTART_SERVICE -eq 0) {
        exit 5
    }

    # Backup current config
    $timestamp = Get-Date -Format "yyyyMMdd-HH:mm:ss"
    Write-ColorOutput " " "White"
    Write-ColorOutput " $timestamp Copying file $script:MYSQLCONFIGURER_CONFIGFILE to directory $script:MYSQL_CONFIG_DIR/." "White"

    $backupFile = "$script:MYSQLCONFIGURER_CONFIGFILE.bkp"
    if (-not (Test-Path $backupFile) -and (Test-Path $currentConfig)) {
        Copy-Item $currentConfig $backupFile -Force
    }

    # Copy new config
    Copy-Item $script:MYSQLCONFIGURER_CONFIGFILE $script:MYSQL_CONFIG_DIR -Force

    # Check restart command
    if (-not $script:MYSQL_RESTART_SERVICE) {
        Write-ColorOutput " " "White"
        Write-ColorOutput " * The command to restart the MySQL service was not found. Try to reinstall Releem Agent." "White"
        exit 4
    }

    # Restart MySQL
    $timestamp = Get-Date -Format "yyyyMMdd-HH:mm:ss"
    Write-ColorOutput " " "White"
    Write-ColorOutput " $timestamp Restarting MySQL with the command '$script:MYSQL_RESTART_SERVICE'." "White"

    try {
        $job = Start-Job -ScriptBlock {
            param($cmd)
            Invoke-Expression $cmd
        } -ArgumentList $script:MYSQL_RESTART_SERVICE

        $RESTART_CODE = Wait-ServiceRestart -ProcessId $job.Id
        Remove-Job -Job $job -Force -ErrorAction SilentlyContinue

        if ($RESTART_CODE -eq 0) {
            $timestamp = Get-Date -Format "yyyyMMdd-HH:mm:ss"
            Write-ColorOutput " " "Green"
            Write-ColorOutput " $timestamp The MySQL service restarted successfully!" "Green"
            Write-ColorOutput " " "Green"
            Write-ColorOutput " $timestamp Recommended configuration applied successfully!" "Green"
            Write-ColorOutput " " "White"
            Write-ColorOutput " $timestamp Releem Score and Unapplied recommendations in the Releem Dashboard will be updated in a few minutes."
            if (Test-Path $backupFile) {
                Remove-Item $backupFile -Force -ErrorAction SilentlyContinue
            }
        }
        elseif ($RESTART_CODE -eq 6) {
            $timestamp = Get-Date -Format "yyyyMMdd-HH:mm:ss"
            Write-ColorOutput " " "Red"
            Write-ColorOutput " $timestamp MySQL service failed to restart in 1200 seconds." "Red"
            Write-ColorOutput " " "Red"
            Write-ColorOutput " $timestamp Wait for the MySQL service to start and Check the MySQL error log." "Red"
            Write-ColorOutput " " "Red"
            Write-ColorOutput " $timestamp Try to roll back the configuration application using the command:" "Red"
            Write-ColorOutput " " "Red"
            Write-ColorOutput " $timestamp $PSCommandPath -r" "Green"
            Write-ColorOutput " " "White"
        }
        elseif ($RESTART_CODE -eq 7) {
            $timestamp = Get-Date -Format "yyyyMMdd-HH:mm:ss"
            Write-ColorOutput " " "Red"
            Write-ColorOutput " $timestamp MySQL service failed to restart! Check the MySQL error log!" "Red"
            Write-ColorOutput " " "Red"
            Write-ColorOutput " $timestamp Try to roll back the configuration application using the command:" "Red"
            Write-ColorOutput " " "Red"
            Write-ColorOutput " $timestamp $PSCommandPath -r" "Green"
            Write-ColorOutput " " "White"
        }

        $timestamp = Get-Date -Format "yyyyMMdd-HH:mm:ss"
        Write-ColorOutput " " "Green"
        Write-ColorOutput " $timestamp Sending notification to Releem Platform." "Green"
        & $script:RELEEM_AGENT_EXE --event=config_applied 2>&1 | Out-Null

        exit $RESTART_CODE
    }
    catch {
        Write-ColorOutput "ERROR applying configuration: $_" "Red"
        exit 7
    }
}

#endregion

#region Main Execution

try {
    # Start logging
    Write-ColorOutput "[$((Get-Date).ToString('yyyy-MM-dd HH:mm:ss'))] MySQL Configurer started" "White"

    # Load configuration from releem.conf
    Load-Configuration

    # Execute requested operation using switch statement
    switch ($true) {
        $UpdateAgent {
            Update-Agent
            break
        }
        $EnablePerformanceSchema {
            Enable-PerformanceSchema
            break
        }
        $ApplyManual {
            Apply-ConfigurationManual
            break
        }
        { $ApplyConfig } {
            if ($ApplyConfig -eq "auto") {
                # Request API to create job
                & $script:RELEEM_AGENT_EXE --task=apply_config 2>&1 | Out-Null
                $timestamp = Get-Date -Format "yyyyMMdd-HH:mm:ss"
                Write-ColorOutput " " "Green"
                Write-ColorOutput " $timestamp Sending request to create a job to apply the configuration." "Green"
            }
            else {
                # Apply automatically (automatic mode)
                Apply-ConfigurationManual
            }
            break
        }
        $Rollback {
            Restore-Configuration
            break
        }
        default {
            # No parameters - show help
            Write-Host ""
            Write-Host " * To run Releem Agent manually please use the following command:" -ForegroundColor White
            Write-Host "   `"$script:RELEEM_AGENT_EXE`" -f" -ForegroundColor Green
            Write-Host ""
        }
    }
}
finally {
    # Upload log to API on exit
    Send-LogToAPI
}

#endregion
