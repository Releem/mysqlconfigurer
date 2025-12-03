#Requires -Version 5.1

<#
.SYNOPSIS
    Releem Agent Installer for Windows

.DESCRIPTION
    Installs, updates, or uninstalls Releem Agent on Windows systems.
    Provides feature parity with install.sh for Linux.

.PARAMETER ApiKey
    Releem API Key (required for installation)

.PARAMETER MysqlHost
    MySQL host address (default: 127.0.0.1)

.PARAMETER MysqlPort
    MySQL port (default: 3306)

.PARAMETER MysqlUser
    MySQL user for metrics collection (default: releem)

.PARAMETER MysqlPassword
    MySQL password for metrics collection

.PARAMETER QueryOptimization
    Enable query optimization (default: true)

.PARAMETER MemoryLimit
    MySQL memory limit in MB (0 = auto-detect)

.PARAMETER Uninstall
    Uninstall Releem Agent

.PARAMETER Update
    Update Releem Agent to latest version

.PARAMETER Silent
    Silent mode - no interactive prompts

.EXAMPLE
    .\install.ps1 -ApiKey "your-api-key" -MysqlPassword "password"

.EXAMPLE
    .\install.ps1 -Update

.EXAMPLE
    .\install.ps1 -Uninstall

.NOTES
    Version: 1.22.0
    Author: Releem, Inc
    Copyright: (C) Releem, Inc 2022-2025
#>

[CmdletBinding()]
Param(
    [Parameter(Mandatory=$false)]
    [string]$ApiKey,

    [Parameter(Mandatory=$false)]
    [string]$MysqlHost = "127.0.0.1",

    [Parameter(Mandatory=$false)]
    [string]$MysqlPort = "3306",

    [Parameter(Mandatory=$false)]
    [string]$MysqlUser = "releem",

    [Parameter(Mandatory=$false)]
    [string]$MysqlPassword,

    [Parameter(Mandatory=$false)]
    [bool]$QueryOptimization = $true,

    [Parameter(Mandatory=$false)]
    [int]$MemoryLimit = 0,

    [Parameter(Mandatory=$false)]
    [string]$Region,

    [Parameter(Mandatory=$false)]
    [switch]$Uninstall,

    [Parameter(Mandatory=$false)]
    [switch]$Update,

    [Parameter(Mandatory=$false)]
    [switch]$Silent
)

#region Script Variables

$script:VERSION = "1.22.0"
$script:INSTALL_PATH = "C:\Program Files\Releem"
$script:DATA_PATH = "C:\ProgramData\Releem"
$script:CONF_PATH = Join-Path $DATA_PATH "conf.d"
$script:RELEEM_CONF = Join-Path $DATA_PATH "releem.conf"
$script:MYSQL_CONF_DIR = Join-Path $DATA_PATH "releem.conf.d"
$script:LOG_FILE = Join-Path $DATA_PATH "logs\releem-install.log"
$script:RELEEM_AGENT_EXE = Join-Path $INSTALL_PATH "releem-agent.exe"
$script:MYSQLCONFIGURER_PS1 = Join-Path $INSTALL_PATH "mysqlconfigurer.ps1"

$script:API_DOMAIN = "api.releem.com"
if ($Region -eq "EU") {
    $script:API_DOMAIN = "api.eu.releem.com"
}

$script:DETECTED_SERVICE = $null
$script:DETECTED_CONFIG_PATH = $null
$script:DETECTED_CONFIG_DIR = $null

#endregion

#region Helper Functions

Function Write-Log {
    Param(
        [Parameter(Mandatory=$true)]
        [string]$Message,

        [Parameter(Mandatory=$false)]
        [ValidateSet('Info', 'Success', 'Warning', 'Error')]
        [string]$Level = 'Info'
    )

    $timestamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
    $logMessage = "[$timestamp] [$Level] $Message"

    # Ensure log directory exists
    $logDir = Split-Path $script:LOG_FILE
    if (-not (Test-Path $logDir)) {
        New-Item -ItemType Directory -Path $logDir -Force | Out-Null
    }

    # Write to log file
    Add-Content -Path $script:LOG_FILE -Value $logMessage -ErrorAction SilentlyContinue

    # Write to console with color
    $color = switch ($Level) {
        'Success' { 'Green' }
        'Warning' { 'Yellow' }
        'Error' { 'Red' }
        default { 'White' }
    }

    Write-Host $Message -ForegroundColor $color
}

Function Send-LogToAPI {
    <#
    .SYNOPSIS
        Upload log file to Releem API
    #>
    Param(
        [Parameter(Mandatory=$false)]
        [string]$Event = "saving_log"
    )

    try {
        if ($ApiKey -and (Test-Path $script:LOG_FILE)) {
            $logContent = Get-Content -Path $script:LOG_FILE -Raw -ErrorAction SilentlyContinue

            if ($logContent) {
                $headers = @{
                    "x-releem-api-key" = $ApiKey
                    "Content-Type" = "application/json"
                }

                $uri = "https://$script:API_DOMAIN/v2/events/$Event"
                Invoke-RestMethod -Uri $uri -Method Post -Body $logContent -Headers $headers -ErrorAction SilentlyContinue | Out-Null
                Write-Log "Installation log uploaded to Releem API" -Level Info
            }
        }
    }
    catch {
        # Silently fail - don't block installation
        Write-Log "Failed to upload log to API: $_" -Level Warning
    }
}

Function Test-Administrator {
    $currentUser = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($currentUser)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

#endregion

#region Detection Functions

Function Detect-MySQLService {
    <#
    .SYNOPSIS
        Detect MySQL/MariaDB Windows service and configuration paths
    #>
    Write-Log "Detecting MySQL/MariaDB service..." -Level Info

    try {
        # Get all services matching MySQL or MariaDB
        $services = Get-Service | Where-Object {
            $_.Name -match 'mysql' -or $_.Name -match 'mariadb'
        }

        if ($services.Count -eq 0) {
            Write-Log "No MySQL/MariaDB service found" -Level Warning
            return $false
        }

        # Prefer running services
        $runningService = $services | Where-Object { $_.Status -eq 'Running' } | Select-Object -First 1
        $selectedService = if ($runningService) { $runningService } else { $services | Select-Object -First 1 }

        $script:DETECTED_SERVICE = $selectedService.Name
        Write-Log "Detected MySQL/MariaDB service: $script:DETECTED_SERVICE" -Level Success

        # Common MySQL configuration paths
        $configPaths = @(
            "C:\ProgramData\MySQL\MySQL Server 8.0\my.ini",
            "C:\ProgramData\MySQL\MySQL Server 8.4\my.ini",
            "C:\ProgramData\MySQL\MySQL Server 5.7\my.ini",
            "C:\Program Files\MySQL\MySQL Server 8.0\my.ini",
            "C:\Program Files\MySQL\MySQL Server 8.4\my.ini",
            "C:\Program Files\MySQL\MySQL Server 5.7\my.ini",
            "C:\Program Files\MariaDB 10.11\data\my.ini",
            "C:\Program Files\MariaDB 11.4\data\my.ini",
            "C:\Program Files\MariaDB 10.6\data\my.ini",
            "C:\Program Files\MariaDB 10.5\data\my.ini",
            "C:\my.ini",
            "C:\mysql\my.ini",
            "C:\my.cnf",
            "C:\mysql\my.cnf"
        )

        foreach ($path in $configPaths) {
            if (Test-Path $path) {
                $script:DETECTED_CONFIG_PATH = $path
                $script:DETECTED_CONFIG_DIR = Split-Path $path -Parent
                Write-Log "Detected MySQL config file: $script:DETECTED_CONFIG_PATH" -Level Success
                return $true
            }
        }

        Write-Log "MySQL config file not found in common locations" -Level Warning

        if (-not $Silent) {
            $customPath = Read-Host "Please enter the path to your MySQL configuration file (my.ini or my.cnf)"
            if ($customPath -and (Test-Path $customPath)) {
                $script:DETECTED_CONFIG_PATH = $customPath
                $script:DETECTED_CONFIG_DIR = Split-Path $customPath -Parent
                Write-Log "Using custom config path: $script:DETECTED_CONFIG_PATH" -Level Success
                return $true
            }
        }

        Write-Log "MySQL configuration file not found. Automatic configuration application will be disabled." -Level Warning
        return $false
    }
    catch {
        Write-Log "Error detecting MySQL service: $_" -Level Error
        return $false
    }
}

#endregion

#region Installation Functions

Function Download-ReleemFiles {
    <#
    .SYNOPSIS
        Download Releem Agent and mysqlconfigurer.ps1 from S3
    #>
    Write-Log "Downloading Releem Agent files..." -Level Info

    try {
        # Create installation directory
        if (-not (Test-Path $script:INSTALL_PATH)) {
            New-Item -ItemType Directory -Path $script:INSTALL_PATH -Force | Out-Null
        }

        # Download releem-agent.exe
        Write-Log "Downloading releem-agent.exe..." -Level Info
        $agentUrl = "https://releem.s3.amazonaws.com/v2/releem-agent.exe"
        Invoke-WebRequest -Uri $agentUrl -OutFile $script:RELEEM_AGENT_EXE -ErrorAction Stop
        Write-Log "Downloaded releem-agent.exe" -Level Success

        # Download mysqlconfigurer.ps1
        Write-Log "Downloading mysqlconfigurer.ps1..." -Level Info
        $configurerUrl = "https://releem.s3.amazonaws.com/v2/mysqlconfigurer.ps1"
        Invoke-WebRequest -Uri $configurerUrl -OutFile $script:MYSQLCONFIGURER_PS1 -ErrorAction Stop
        Write-Log "Downloaded mysqlconfigurer.ps1" -Level Success

        return $true
    }
    catch {
        Write-Log "Failed to download Releem files: $_" -Level Error
        return $false
    }
}

Function Add-MySQLIncludeDir {
    <#
    .SYNOPSIS
        Add !includedir directive to MySQL configuration file
    #>
    Write-Log "Configuring MySQL to load Releem configurations..." -Level Info

    if (-not $script:DETECTED_CONFIG_PATH) {
        Write-Log "MySQL config path not detected. Skipping includedir configuration." -Level Warning
        return $false
    }

    try {
        # Create Releem config directory
        if (-not (Test-Path $script:MYSQL_CONF_DIR)) {
            New-Item -ItemType Directory -Path $script:MYSQL_CONF_DIR -Force | Out-Null
            Write-Log "Created Releem config directory: $script:MYSQL_CONF_DIR" -Level Success
        }

        # Read current MySQL config
        $configContent = Get-Content -Path $script:DETECTED_CONFIG_PATH -Raw -ErrorAction Stop

        # Check if includedir already exists
        $includeDirective = "!includedir $script:MYSQL_CONF_DIR"

        if ($configContent -match [regex]::Escape($includeDirective)) {
            Write-Log "MySQL config already includes Releem directory" -Level Info
            return $true
        }

        # Backup original config
        $backupPath = "$script:DETECTED_CONFIG_PATH.releem.bkp"
        if (-not (Test-Path $backupPath)) {
            Copy-Item -Path $script:DETECTED_CONFIG_PATH -Destination $backupPath -Force
            Write-Log "Created backup: $backupPath" -Level Success
        }

        # Add includedir directive
        Write-Log "Adding !includedir directive to MySQL configuration..." -Level Info
        $newContent = $configContent.TrimEnd() + "`n`n# Releem configuration directory`n$includeDirective`n"
        Set-Content -Path $script:DETECTED_CONFIG_PATH -Value $newContent -Force -ErrorAction Stop
        Write-Log "Successfully added !includedir directive" -Level Success

        return $true
    }
    catch {
        Write-Log "Failed to modify MySQL config: $_" -Level Error
        return $false
    }
}

Function New-ReleemConfiguration {
    <#
    .SYNOPSIS
        Generate releem.conf configuration file
    #>
    Write-Log "Generating Releem configuration..." -Level Info

    try {
        # Create data directory
        if (-not (Test-Path $script:DATA_PATH)) {
            New-Item -ItemType Directory -Path $script:DATA_PATH -Force | Out-Null
        }

        if (-not (Test-Path $script:CONF_PATH)) {
            New-Item -ItemType Directory -Path $script:CONF_PATH -Force | Out-Null
        }

        # Build configuration content
        $configLines = @()
        $configLines += "# Releem Agent Configuration"
        $configLines += "# Generated by install.ps1 on $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')"
        $configLines += ""
        $configLines += "apikey=`"$ApiKey`""
        $configLines += "releem_cnf_dir=`"$script:CONF_PATH`""
        $configLines += "mysql_host=`"$MysqlHost`""
        $configLines += "mysql_port=`"$MysqlPort`""
        $configLines += "mysql_user=`"$MysqlUser`""
        $configLines += "mysql_password=`"$MysqlPassword`""

        if ($MemoryLimit -gt 0) {
            $configLines += "memory_limit=`"$MemoryLimit`""
        }

        if ($script:DETECTED_SERVICE) {
            $restartCmd = "net stop `"$script:DETECTED_SERVICE`" && net start `"$script:DETECTED_SERVICE`""
            $configLines += "mysql_restart_service=`"$restartCmd`""
        }

        if ($script:MYSQL_CONF_DIR) {
            $configLines += "mysql_cnf_dir=`"$script:MYSQL_CONF_DIR`""
        }

        $configLines += "hostname=`"$env:COMPUTERNAME`""
        $configLines += "query_optimization=`"$QueryOptimization`""

        if ($Region) {
            $configLines += "releem_region=`"$Region`""
        }

        # Write configuration file
        $configContent = $configLines -join "`n"
        Set-Content -Path $script:RELEEM_CONF -Value $configContent -Force -ErrorAction Stop

        Write-Log "Configuration saved to $script:RELEEM_CONF" -Level Success
        return $true
    }
    catch {
        Write-Log "Failed to create configuration: $_" -Level Error
        return $false
    }
}

Function Install-ReleemService {
    <#
    .SYNOPSIS
        Install and start Releem Agent Windows service
    #>
    Write-Log "Installing Releem Agent service..." -Level Info

    try {
        # Remove existing service if present
        & $script:RELEEM_AGENT_EXE remove 2>&1 | Out-Null

        # Install service
        $installResult = & $script:RELEEM_AGENT_EXE install 2>&1
        if ($LASTEXITCODE -ne 0) {
            Write-Log "Service installation returned exit code $LASTEXITCODE" -Level Warning
        } else {
            Write-Log "Releem Agent service installed successfully" -Level Success
        }

        # Start service
        Write-Log "Starting Releem Agent service..." -Level Info
        $startResult = & $script:RELEEM_AGENT_EXE start 2>&1
        if ($LASTEXITCODE -ne 0) {
            Write-Log "Service start returned exit code $LASTEXITCODE" -Level Warning
        } else {
            Write-Log "Releem Agent service started successfully" -Level Success
        }

        return $true
    }
    catch {
        Write-Log "Failed to install service: $_" -Level Error
        return $false
    }
}

Function Initialize-ReleemAgent {
    <#
    .SYNOPSIS
        Run agent first time and enable performance schema
    #>
    Write-Log "Executing Releem Agent for the first time..." -Level Info
    Write-Log "This may take up to 15 minutes on servers with many databases." -Level Info

    try {
        # First run to collect metrics
        & $script:RELEEM_AGENT_EXE -f 2>&1 | Out-Null
        Write-Log "Initial metrics collection completed" -Level Success

        # Enable performance schema
        Write-Log "Enabling Performance Schema and Slow Query Log..." -Level Info
        & $script:MYSQLCONFIGURER_PS1 -p 2>&1 | Out-Null
        Write-Log "Performance Schema configuration completed" -Level Success

        return $true
    }
    catch {
        Write-Log "Failed during initialization: $_" -Level Error
        return $false
    }
}

Function New-UpdateScheduledTask {
    <#
    .SYNOPSIS
        Create scheduled task for daily updates (equivalent to cron job)
    #>
    Write-Log "Creating scheduled task for automatic updates..." -Level Info

    try {
        $taskName = "ReleemAgentUpdate"
        $taskDescription = "Daily Releem Agent update check"

        # Remove existing task if present
        Unregister-ScheduledTask -TaskName $taskName -Confirm:$false -ErrorAction SilentlyContinue

        # Create action
        $action = New-ScheduledTaskAction -Execute "PowerShell.exe" `
            -Argument "-ExecutionPolicy Bypass -NoProfile -WindowStyle Hidden -File `"$script:MYSQLCONFIGURER_PS1`" -u"

        # Create trigger (daily at midnight)
        $trigger = New-ScheduledTaskTrigger -Daily -At "00:00"

        # Create settings
        $settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable

        # Create principal (run as SYSTEM)
        $principal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -LogonType ServiceAccount -RunLevel Highest

        # Register task
        Register-ScheduledTask -TaskName $taskName `
            -Description $taskDescription `
            -Action $action `
            -Trigger $trigger `
            -Settings $settings `
            -Principal $principal `
            -ErrorAction Stop | Out-Null

        Write-Log "Scheduled task created successfully" -Level Success
        Write-Log "Automatic updates will run daily at midnight" -Level Info

        return $true
    }
    catch {
        Write-Log "Failed to create scheduled task: $_" -Level Error
        return $false
    }
}

Function Test-Installation {
    <#
    .SYNOPSIS
        Validate that installation completed successfully
    #>
    Write-Log "Validating installation..." -Level Info

    try {
        # Check if files exist
        if (-not (Test-Path $script:RELEEM_AGENT_EXE)) {
            Write-Log "Agent executable not found" -Level Error
            return $false
        }

        if (-not (Test-Path $script:MYSQLCONFIGURER_PS1)) {
            Write-Log "MySQL configurer script not found" -Level Error
            return $false
        }

        if (-not (Test-Path $script:RELEEM_CONF)) {
            Write-Log "Configuration file not found" -Level Error
            return $false
        }

        # Check if service is running
        Start-Sleep -Seconds 3
        $process = Get-Process -Name "releem-agent" -ErrorAction SilentlyContinue
        if (-not $process) {
            Write-Log "Releem Agent process not running" -Level Warning
        } else {
            Write-Log "Releem Agent is running (PID: $($process.Id))" -Level Success
        }

        # Check if scheduled task exists
        $task = Get-ScheduledTask -TaskName "ReleemAgentUpdate" -ErrorAction SilentlyContinue
        if ($task) {
            Write-Log "Scheduled task configured" -Level Success
        } else {
            Write-Log "Scheduled task not found" -Level Warning
        }

        Write-Log "Installation validation completed" -Level Success
        return $true
    }
    catch {
        Write-Log "Validation failed: $_" -Level Error
        return $false
    }
}

#endregion

#region Uninstall Functions

Function Uninstall-ReleemAgent {
    <#
    .SYNOPSIS
        Uninstall Releem Agent and clean up all files
    #>
    Write-Log "Uninstalling Releem Agent..." -Level Info

    try {
        # Send uninstall event
        if (Test-Path $script:RELEEM_AGENT_EXE) {
            & $script:RELEEM_AGENT_EXE --event=agent_uninstall 2>&1 | Out-Null
        }

        # Remove scheduled task
        Write-Log "Removing scheduled task..." -Level Info
        Unregister-ScheduledTask -TaskName "ReleemAgentUpdate" -Confirm:$false -ErrorAction SilentlyContinue

        # Stop and remove service
        if (Test-Path $script:RELEEM_AGENT_EXE) {
            Write-Log "Stopping Releem Agent service..." -Level Info
            & $script:RELEEM_AGENT_EXE stop 2>&1 | Out-Null

            Write-Log "Removing Releem Agent service..." -Level Info
            & $script:RELEEM_AGENT_EXE remove 2>&1 | Out-Null
        }

        # Remove includedir from MySQL config
        if ($script:DETECTED_CONFIG_PATH -and (Test-Path $script:DETECTED_CONFIG_PATH)) {
            Write-Log "Removing !includedir directive from MySQL configuration..." -Level Info
            $configContent = Get-Content -Path $script:DETECTED_CONFIG_PATH -Raw
            $includeDirective = "!includedir $script:MYSQL_CONF_DIR"

            if ($configContent -match [regex]::Escape($includeDirective)) {
                $newContent = $configContent -replace "(?m)^\s*#\s*Releem configuration directory\s*$", ""
                $newContent = $newContent -replace "(?m)^\s*!includedir.*releem\.conf\.d.*$", ""
                Set-Content -Path $script:DETECTED_CONFIG_PATH -Value $newContent.TrimEnd() -Force
                Write-Log "Removed !includedir directive" -Level Success
            }
        }

        # Remove files
        Write-Log "Removing installation files..." -Level Info
        if (Test-Path $script:INSTALL_PATH) {
            Remove-Item -Path $script:INSTALL_PATH -Recurse -Force -ErrorAction SilentlyContinue
        }

        if (Test-Path $script:DATA_PATH) {
            Remove-Item -Path $script:DATA_PATH -Recurse -Force -ErrorAction SilentlyContinue
        }

        Write-Log "Releem Agent uninstalled successfully" -Level Success
        return $true
    }
    catch {
        Write-Log "Uninstall failed: $_" -Level Error
        return $false
    }
}

#endregion

#region Update Functions

Function Update-ReleemAgent {
    <#
    .SYNOPSIS
        Update Releem Agent to latest version
    #>
    Write-Log "Updating Releem Agent..." -Level Info

    try {
        # Check current version
        $currentVersion = $script:VERSION
        $newVersion = (Invoke-RestMethod -Uri "https://releem.s3.amazonaws.com/v2/current_version_agent" -TimeoutSec 10).Trim()

        Write-Log "Current version: $currentVersion" -Level Info
        Write-Log "Latest version: $newVersion" -Level Info

        if ($newVersion -eq $currentVersion) {
            Write-Log "Already running latest version" -Level Success
            return $true
        }

        # Stop agent
        Write-Log "Stopping Releem Agent..." -Level Info
        & $script:RELEEM_AGENT_EXE stop 2>&1 | Out-Null
        Start-Sleep -Seconds 2

        # Download new version
        Write-Log "Downloading new version..." -Level Info
        $tempExe = "$script:RELEEM_AGENT_EXE.new"
        Invoke-WebRequest -Uri "https://releem.s3.amazonaws.com/v2/releem-agent-windows.exe" -OutFile $tempExe -ErrorAction Stop

        # Replace executable
        Move-Item -Path $tempExe -Destination $script:RELEEM_AGENT_EXE -Force

        # Download updated mysqlconfigurer.ps1
        try {
            $tempScript = "$script:MYSQLCONFIGURER_PS1.new"
            Invoke-WebRequest -Uri "https://releem.s3.amazonaws.com/v2/mysqlconfigurer.ps1" -OutFile $tempScript -ErrorAction Stop
            Move-Item -Path $tempScript -Destination $script:MYSQLCONFIGURER_PS1 -Force
            Write-Log "Updated mysqlconfigurer.ps1" -Level Success
        }
        catch {
            Write-Log "No script update available" -Level Info
        }

        # Start agent
        Write-Log "Starting Releem Agent..." -Level Info
        & $script:RELEEM_AGENT_EXE start 2>&1 | Out-Null

        # Run once
        & $script:RELEEM_AGENT_EXE -f 2>&1 | Out-Null

        # Send event
        & $script:RELEEM_AGENT_EXE --event=agent_updated 2>&1 | Out-Null

        Write-Log "Releem Agent updated successfully to version $newVersion" -Level Success
        return $true
    }
    catch {
        Write-Log "Update failed: $_" -Level Error
        return $false
    }
}

#endregion

#region Main Execution

try {
    Write-Log "========================================" -Level Info
    Write-Log "Releem Agent Installer v$script:VERSION" -Level Info
    Write-Log "========================================" -Level Info

    # Check administrator privileges
    if (-not (Test-Administrator)) {
        Write-Log "ERROR: This script must be run as Administrator" -Level Error
        exit 1
    }

    # Handle different modes
    if ($Uninstall) {
        # Detect MySQL service for uninstall (to remove includedir)
        Detect-MySQLService | Out-Null

        $success = Uninstall-ReleemAgent
        Send-LogToAPI -Event "agent_uninstall"
        exit $(if ($success) { 0 } else { 1 })
    }

    if ($Update) {
        # Load configuration to get API key
        if (Test-Path $script:RELEEM_CONF) {
            Get-Content $script:RELEEM_CONF | ForEach-Object {
                if ($_ -match '^\s*apikey\s*=\s*"?([^"]*)"?\s*$') {
                    $script:ApiKey = $matches[1].Trim().Trim('"')
                }
            }
        }

        $success = Update-ReleemAgent
        Send-LogToAPI -Event "agent_updated"
        exit $(if ($success) { 0 } else { 1 })
    }

    # Installation mode
    if (-not $ApiKey) {
        Write-Log "ERROR: API key is required for installation" -Level Error
        Write-Log "Please sign up at https://releem.com to get your API key" -Level Error
        exit 1
    }

    # Prompt for memory limit if not provided and not silent
    if ($MemoryLimit -eq 0 -and -not $Silent) {
        Write-Host ""
        Write-Host "In case you are using MySQL in Docker or it isn't dedicated server for MySQL." -ForegroundColor White
        $response = Read-Host "Should we limit memory for MySQL database? (Y/N)"
        if ($response -match '^[Yy]') {
            $limitInput = Read-Host "Please set MySQL Memory Limit (megabytes)"
            if ($limitInput) {
                $MemoryLimit = [int]$limitInput
            }
        }
    }

    Write-Host ""
    Write-Log "Starting installation with following parameters:" -Level Info
    Write-Log "  MySQL Host: $MysqlHost" -Level Info
    Write-Log "  MySQL Port: $MysqlPort" -Level Info
    Write-Log "  MySQL User: $MysqlUser" -Level Info
    Write-Log "  Query Optimization: $QueryOptimization" -Level Info
    if ($MemoryLimit -gt 0) {
        Write-Log "  Memory Limit: $MemoryLimit MB" -Level Info
    }
    Write-Host ""

    # Execute installation steps
    $steps = @(
        @{ Name = "Detect MySQL/MariaDB service"; Function = { Detect-MySQLService } },
        @{ Name = "Download Releem files"; Function = { Download-ReleemFiles } },
        @{ Name = "Add MySQL includedir"; Function = { Add-MySQLIncludeDir } },
        @{ Name = "Generate configuration"; Function = { New-ReleemConfiguration } },
        @{ Name = "Install service"; Function = { Install-ReleemService } },
        @{ Name = "Initialize agent"; Function = { Initialize-ReleemAgent } },
        @{ Name = "Create scheduled task"; Function = { New-UpdateScheduledTask } },
        @{ Name = "Validate installation"; Function = { Test-Installation } }
    )

    $allSuccess = $true
    foreach ($step in $steps) {
        Write-Host ""
        $result = & $step.Function
        if (-not $result) {
            Write-Log "Warning: Step '$($step.Name)' encountered issues but continuing..." -Level Warning
            # Don't fail on non-critical steps
            if ($step.Name -match "Detect|Download|Generate|Install service") {
                $allSuccess = $false
                break
            }
        }
    }

    # Upload installation log
    Send-LogToAPI -Event "saving_log"

    if ($allSuccess) {
        Write-Host ""
        Write-Log "========================================" -Level Success
        Write-Log "Releem Agent installed successfully!" -Level Success
        Write-Log "========================================" -Level Success
        Write-Host ""
        Write-Log "To view Releem recommendations and MySQL metrics, visit:" -Level Info
        Write-Log "https://app.releem.com/dashboard" -Level Success
        Write-Host ""
        exit 0
    } else {
        Write-Host ""
        Write-Log "Installation encountered errors. Please check the log at:" -Level Error
        Write-Log $script:LOG_FILE -Level Error
        Write-Host ""
        Write-Log "If you need assistance, please contact hello@releem.com" -Level Error
        exit 1
    }
}
catch {
    Write-Log "Unhandled error during installation: $_" -Level Error
    Write-Log $_.ScriptStackTrace -Level Error
    Send-LogToAPI -Event "saving_log"
    exit 1
}

#endregion
