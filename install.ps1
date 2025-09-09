# install.ps1 - Version 1.22.0
# (C) Releem, Inc 2022
# All rights reserved

# Releem installation script: install and set up the Releem Agent on Windows
# PowerShell version with enhanced functionality

param(
    [switch]$Update,
    [switch]$Uninstall,
    [switch]$EnableQueryOptimization,
    [string]$ApiKey,
    [string]$MySQLHost,
    [string]$MySQLPort,
    [string]$MySQLRootPassword,
    [string]$MySQLLogin,
    [string]$MySQLPassword,
    [string]$Region,
    [switch]$SkipCron,
    [switch]$DisableAgent,
    [int]$MemoryLimit
)

$ErrorActionPreference = "Stop"
$INSTALL_SCRIPT_VERSION = "1.22.0"
$LOGFILE = "$env:TEMP\releem-install.log"

$WORKDIR = "C:\Program Files\ReleemAgent"
$CONF = "$WORKDIR\releem.conf"
$MYSQL_CONF_DIR = "$WORKDIR\conf"

# Function to log messages
function Write-Log {
    param([string]$Message)
    $timestamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
    $logMessage = "[$timestamp] $Message"
    Write-Host $Message
    Add-Content -Path $LOGFILE -Value $logMessage
}

# Function to send log to API
function Send-LogToAPI {
    param([string]$ApiKey)
    
    if ($env:RELEEM_REGION -eq "EU") {
        $apiDomain = "api.eu.releem.com"
    } else {
        $apiDomain = "api.releem.com"
    }
    
    try {
        $headers = @{
            'x-releem-api-key' = $ApiKey
            'Content-Type' = 'application/json'
        }
        
        $logContent = Get-Content -Path $LOGFILE -Raw
        Invoke-RestMethod -Uri "https://$apiDomain/v2/events/saving_log" -Method Post -Body $logContent -Headers $headers
    }
    catch {
        Write-Log "Failed to send log to API: $($_.Exception.Message)"
    }
}

# Function to handle errors
function Handle-Error {
    param([string]$ErrorMessage)
    
    Write-Log "ERROR: $ErrorMessage"
    Write-Log "It looks like you encountered an issue while installing Releem."
    Write-Log "If you are still experiencing problems, please send an email to hello@releem.com"
    Write-Log "with the contents of $LOGFILE. We will do our best to resolve the issue."
    
    if ($script:ApiKey) {
        Send-LogToAPI -ApiKey $script:ApiKey
    }
    
    exit 1
}

# Function to update Releem Agent
function Update-ReleemAgent {
    Write-Log "* Downloading latest version of Releem Agent."
    
    # Stop existing agent
    if (Test-Path "$WORKDIR\releem-agent.exe") {
        Write-Log "Stopping existing Releem Agent..."
        try {
            & "$WORKDIR\releem-agent.exe" stop
        }
        catch {
            Write-Log "Warning: Could not stop existing agent"
        }
    }
    
    # Download new version
    try {
        Invoke-WebRequest -Uri 'https://releem.s3.amazonaws.com/v2/releem-agent-windows.exe' -OutFile "$WORKDIR\releem-agent.new"
        Invoke-WebRequest -Uri 'https://releem.s3.amazonaws.com/v2/mysqlconfigurer.ps1' -OutFile "$WORKDIR\mysqlconfigurer.ps1.new"
        
        # Replace files
        Move-Item "$WORKDIR\releem-agent.new" "$WORKDIR\releem-agent.exe" -Force
        Move-Item "$WORKDIR\mysqlconfigurer.ps1.new" "$WORKDIR\mysqlconfigurer.ps1" -Force
        
        # Start the service
        & "$WORKDIR\releem-agent.exe" start
        & "$WORKDIR\releem-agent.exe" -f
        
        Write-Log ""
        Write-Log "Releem Agent updated successfully."
        Write-Log ""
        Write-Log "To check MySQL Performance Score please visit https://app.releem.com/dashboard?menu=metrics"
        Write-Log ""
        exit 0
    }
    catch {
        Handle-Error "Failed to update Releem Agent: $($_.Exception.Message)"
    }
}

# Function to uninstall Releem Agent
function Uninstall-ReleemAgent {
    Write-Log "* Uninstalling Releem Agent"
    
    # Send uninstall event
    if (Test-Path "$WORKDIR\releem-agent.exe") {
        try {
            & "$WORKDIR\releem-agent.exe" --event=agent_uninstall
        }
        catch {
            Write-Log "Warning: Could not send uninstall event"
        }
    }
    
    Write-Log "* Removing scheduled task"
    try {
        Unregister-ScheduledTask -TaskName "Releem Agent Config Update" -Confirm:$false -ErrorAction SilentlyContinue
    }
    catch {
        Write-Log "Warning: Could not remove scheduled task"
    }
    
    Write-Log "* Stopping Releem Agent service"
    if (Test-Path "$WORKDIR\releem-agent.exe") {
        try {
            & "$WORKDIR\releem-agent.exe" stop
            Write-Log "Releem Agent stopped successfully."
        }
        catch {
            Write-Log "Releem Agent failed to stop."
        }
    }
    
    Write-Log "* Uninstalling Releem Agent service"
    if (Test-Path "$WORKDIR\releem-agent.exe") {
        try {
            & "$WORKDIR\releem-agent.exe" remove
            Write-Log "Releem Agent uninstalled successfully."
        }
        catch {
            Write-Log "Releem Agent failed to uninstall."
        }
    }
    
    Write-Log "* Removing Releem files"
    try {
        Remove-Item -Path $WORKDIR -Recurse -Force -ErrorAction SilentlyContinue
    }
    catch {
        Write-Log "Warning: Could not remove all files"
    }
    
    exit 0
}

# Function to enable query optimization
function Enable-QueryOptimization {
    Write-Log "* Enabling Query Optimization"
    
    # Grant SELECT privileges to releem user
    $grantQueries = & $script:mysqlcmd $script:root_connection_string --user=root --password=$env:RELEEM_MYSQL_ROOT_PASSWORD -NBe "select Concat('GRANT SELECT on *.* to `',User,'`@`', Host,'`;') from mysql.user where User='releem'"
    
    foreach ($query in $grantQueries) {
        Write-Log "Executing: $query"
        & $script:mysqlcmd $script:root_connection_string --user=root --password=$env:RELEEM_MYSQL_ROOT_PASSWORD -Be $query
    }
    
    # Add query optimization to config if not present
    if (-not (Get-Content $CONF | Select-String "query_optimization")) {
        Add-Content -Path $CONF -Value "query_optimization=true"
    }
    
    # Restart Releem Agent
    Write-Log "* Restarting Releem Agent"
    try {
        & "$WORKDIR\releem-agent.exe" stop
        & "$WORKDIR\releem-agent.exe" start
        Write-Log "Restarting Releem Agent - successful"
    }
    catch {
        Write-Log "Restarting Releem Agent - failed"
    }
    
    Start-Sleep -Seconds 3
    
    # Check if process is running
    if (-not (Get-Process -Name "releem-agent" -ErrorAction SilentlyContinue)) {
        Handle-Error "The releem-agent process was not found! Check the system log for an error."
    }
    
    # Enable performance schema
    & "$WORKDIR\mysqlconfigurer.ps1" -p
    exit 0
}

# Function to find MySQL executables
function Find-MySQLExecutables {
    $mysqladmincmd = $null
    $mysqlcmd = $null
    
    # Common MySQL installation paths
    $commonPaths = @(
        "C:\Program Files\MySQL\MySQL Server 8.0\bin",
        "C:\Program Files\MySQL\MySQL Server 5.7\bin",
        "C:\Program Files (x86)\MySQL\MySQL Server 8.0\bin",
        "C:\Program Files (x86)\MySQL\MySQL Server 5.7\bin",
        "C:\mysql\bin",
        "C:\xampp\mysql\bin"
    )
    
    foreach ($path in $commonPaths) {
        if (Test-Path "$path\mysqladmin.exe") {
            $mysqladmincmd = "$path\mysqladmin.exe"
            break
        }
    }
    
    foreach ($path in $commonPaths) {
        if (Test-Path "$path\mysql.exe") {
            $mysqlcmd = "$path\mysql.exe"
            break
        }
    }
    
    # Try to find in PATH
    if (-not $mysqladmincmd) {
        try {
            $mysqladmincmd = (Get-Command mysqladmin.exe -ErrorAction Stop).Source
        }
        catch {
            # Not found in PATH
        }
    }
    
    if (-not $mysqlcmd) {
        try {
            $mysqlcmd = (Get-Command mysql.exe -ErrorAction Stop).Source
        }
        catch {
            # Not found in PATH
        }
    }
    
    if (-not $mysqladmincmd) {
        Handle-Error "Couldn't find mysqladmin.exe. Please ensure MySQL is installed and accessible."
    }
    
    if (-not $mysqlcmd) {
        Handle-Error "Couldn't find mysql.exe. Please ensure MySQL is installed and accessible."
    }
    
    return @{
        mysqladmin = $mysqladmincmd
        mysql = $mysqlcmd
    }
}

# Function to test MySQL connection
function Test-MySQLConnection {
    param(
        [string]$Command,
        [string]$ConnectionString,
        [string]$User,
        [string]$Password
    )
    
    try {
        $result = & $Command $ConnectionString --user=$User --password=$Password ping 2>$null
        return $result -eq "mysqld is alive"
    }
    catch {
        return $false
    }
}

# Function to generate random password
function New-RandomPassword {
    $chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!@#$%^&*"
    $password = ""
    for ($i = 0; $i -lt 16; $i++) {
        $password += $chars[(Get-Random -Maximum $chars.Length)]
    }
    return "Releem_$password"
}

# Main installation logic
function Install-ReleemAgent {
    Write-Log "Starting Releem installation at $(Get-Date)"
    
    # Handle command line arguments
    if ($Update) {
        Update-ReleemAgent
        return
    }
    
    if ($Uninstall) {
        Uninstall-ReleemAgent
        return
    }
    
    if ($EnableQueryOptimization) {
        Enable-QueryOptimization
        return
    }
    
    # Check for API key
    $script:ApiKey = $ApiKey
    if (-not $script:ApiKey) {
        $script:ApiKey = $env:RELEEM_API_KEY
    }
    
    if (-not $script:ApiKey -and (Test-Path $CONF)) {
        $configContent = Get-Content $CONF
        $apiKeyLine = $configContent | Select-String "apikey="
        if ($apiKeyLine) {
            $script:ApiKey = ($apiKeyLine -split "=")[1].Trim('"')
        }
    }
    
    if (-not $script:ApiKey) {
        Handle-Error "Releem API key is not available. Please sign up at https://releem.com and provide the API key."
    }
    
    Write-Log "* Checking for MySQL installation"
    
    # Find MySQL executables
    $mysqlExes = Find-MySQLExecutables
    $script:mysqladmincmd = $mysqlExes.mysqladmin
    $script:mysqlcmd = $mysqlExes.mysql
    
    Write-Log "* Found MySQL at: $($script:mysqlcmd)"
    Write-Log "* Found mysqladmin at: $($script:mysqladmincmd)"
    
    # Build connection string
    $script:connection_string = ""
    $script:root_connection_string = ""
    
    $hostToUse = $MySQLHost
    if (-not $hostToUse) {
        $hostToUse = $env:RELEEM_MYSQL_HOST
    }
    if (-not $hostToUse) {
        $hostToUse = "127.0.0.1"
    }
    
    $portToUse = $MySQLPort
    if (-not $portToUse) {
        $portToUse = $env:RELEEM_MYSQL_PORT
    }
    if (-not $portToUse) {
        $portToUse = "3306"
    }
    
    if ($hostToUse -eq "127.0.0.1" -or $hostToUse -eq "localhost") {
        $script:mysql_user_host = $hostToUse
    } else {
        $script:mysql_user_host = "%"
    }
    
    $script:connection_string = "--host=$hostToUse --port=$portToUse"
    $script:root_connection_string = "--host=$hostToUse --port=$portToUse"
    
    Write-Log "* Creating work directory"
    if (-not (Test-Path $WORKDIR)) {
        New-Item -Path $WORKDIR -ItemType Directory -Force | Out-Null
    }
    if (-not (Test-Path "$WORKDIR\conf")) {
        New-Item -Path "$WORKDIR\conf" -ItemType Directory -Force | Out-Null
    }
    
    Write-Log "* Downloading Releem Agent for Windows"
    try {
        Invoke-WebRequest -Uri 'https://releem.s3.amazonaws.com/v2/mysqlconfigurer.ps1' -OutFile "$WORKDIR\mysqlconfigurer.ps1"
        Invoke-WebRequest -Uri 'https://releem.s3.amazonaws.com/v2/releem-agent-windows.exe' -OutFile "$WORKDIR\releem-agent.exe"
    }
    catch {
        Handle-Error "Failed to download Releem Agent: $($_.Exception.Message)"
    }
    
    Write-Log "* Configuring MySQL user for metrics collection"
    
    $mysqlLogin = $MySQLLogin
    $mysqlPassword = $MySQLPassword
    $rootPassword = $MySQLRootPassword
    
    if (-not $mysqlLogin) { $mysqlLogin = $env:RELEEM_MYSQL_LOGIN }
    if (-not $mysqlPassword) { $mysqlPassword = $env:RELEEM_MYSQL_PASSWORD }
    if (-not $rootPassword) { $rootPassword = $env:RELEEM_MYSQL_ROOT_PASSWORD }
    
    $flagSuccess = $false
    
    if ($mysqlLogin -and $mysqlPassword) {
        Write-Log "* Using MySQL login and password from parameters/environment variables"
        $flagSuccess = $true
    }
    else {
        Write-Log "* Using MySQL root user"
        
        if (-not $rootPassword) {
            $rootPassword = Read-Host -Prompt "Please enter MySQL root password" -AsSecureString
            $rootPassword = [System.Runtime.InteropServices.Marshal]::PtrToStringAuto([System.Runtime.InteropServices.Marshal]::SecureStringToBSTR($rootPassword))
        }
        
        # Test MySQL connection
        if (Test-MySQLConnection -Command $script:mysqladmincmd -ConnectionString $script:root_connection_string -User "root" -Password $rootPassword) {
            Write-Log "MySQL connection successful."
            
            $mysqlLogin = "releem"
            $mysqlPassword = New-RandomPassword
            
            # Create MySQL user
            try {
                & $script:mysqlcmd $script:root_connection_string --user=root --password=$rootPassword -Be "DROP USER IF EXISTS '$mysqlLogin'@'$($script:mysql_user_host)';" 2>$null
                & $script:mysqlcmd $script:root_connection_string --user=root --password=$rootPassword -Be "CREATE USER '$mysqlLogin'@'$($script:mysql_user_host)' identified by '$mysqlPassword';"
                
                # Grant privileges
                & $script:mysqlcmd $script:root_connection_string --user=root --password=$rootPassword -Be "GRANT PROCESS ON *.* TO '$mysqlLogin'@'$($script:mysql_user_host)';"
                & $script:mysqlcmd $script:root_connection_string --user=root --password=$rootPassword -Be "GRANT REPLICATION CLIENT ON *.* TO '$mysqlLogin'@'$($script:mysql_user_host)';"
                & $script:mysqlcmd $script:root_connection_string --user=root --password=$rootPassword -Be "GRANT SHOW VIEW ON *.* TO '$mysqlLogin'@'$($script:mysql_user_host)';"
                & $script:mysqlcmd $script:root_connection_string --user=root --password=$rootPassword -Be "GRANT SELECT ON mysql.* TO '$mysqlLogin'@'$($script:mysql_user_host)';"
                
                # Grant performance schema privileges (ignore errors for older versions)
                try { & $script:mysqlcmd $script:root_connection_string --user=root --password=$rootPassword -Be "GRANT SELECT ON performance_schema.events_statements_summary_by_digest TO '$mysqlLogin'@'$($script:mysql_user_host)';" 2>$null } catch {}
                try { & $script:mysqlcmd $script:root_connection_string --user=root --password=$rootPassword -Be "GRANT SELECT ON performance_schema.table_io_waits_summary_by_index_usage TO '$mysqlLogin'@'$($script:mysql_user_host)';" 2>$null } catch {}
                try { & $script:mysqlcmd $script:root_connection_string --user=root --password=$rootPassword -Be "GRANT SELECT ON performance_schema.file_summary_by_instance TO '$mysqlLogin'@'$($script:mysql_user_host)';" 2>$null } catch {}
                
                # Grant SUPER or SYSTEM_VARIABLES_ADMIN privilege
                try {
                    & $script:mysqlcmd $script:root_connection_string --user=root --password=$rootPassword -Be "GRANT SYSTEM_VARIABLES_ADMIN ON *.* TO '$mysqlLogin'@'$($script:mysql_user_host)';" 2>$null
                }
                catch {
                    try {
                        & $script:mysqlcmd $script:root_connection_string --user=root --password=$rootPassword -Be "GRANT SUPER ON *.* TO '$mysqlLogin'@'$($script:mysql_user_host)';" 2>$null
                    }
                    catch {
                        Write-Log "Warning: Could not grant SUPER or SYSTEM_VARIABLES_ADMIN privileges"
                    }
                }
                
                if ($env:RELEEM_QUERY_OPTIMIZATION) {
                    & $script:mysqlcmd $script:root_connection_string --user=root --password=$rootPassword -Be "GRANT SELECT ON *.* TO '$mysqlLogin'@'$($script:mysql_user_host)';"
                }
                
                Write-Log "Created new user `$mysqlLogin`"
                $flagSuccess = $true
            }
            catch {
                Handle-Error "Failed to create MySQL user: $($_.Exception.Message)"
            }
        }
        else {
            Handle-Error "MySQL connection failed with user root. Check that the password is correct."
        }
    }
    
    if ($flagSuccess) {
        # Test connection with created user
        if (Test-MySQLConnection -Command $script:mysqladmincmd -ConnectionString $script:connection_string -User $mysqlLogin -Password $mysqlPassword) {
            Write-Log "MySQL connection with user `$mysqlLogin` - successful."
        }
        else {
            Handle-Error "MySQL connection failed with user `$mysqlLogin`. Check that the user and password are correct."
        }
    }
    
    Write-Log "* Configuring MySQL memory limit"
    $mysqlLimit = $MemoryLimit
    if (-not $mysqlLimit) {
        $mysqlLimit = $env:RELEEM_MYSQL_MEMORY_LIMIT
    }
    
    if (-not $mysqlLimit) {
        Write-Log "In case you are using MySQL in Docker or it isn't a dedicated server for MySQL."
        $reply = Read-Host "Should we limit memory for MySQL database? (Y/N)"
        if ($reply -eq "Y" -or $reply -eq "y") {
            $mysqlLimit = Read-Host "Please set MySQL Memory Limit (megabytes)"
        }
    }
    
    Write-Log "* Saving variables to Releem Agent configuration"
    
    # Create configuration file
    $configLines = @(
        "apikey=`"$($script:ApiKey)`""
    )
    
    if (Test-Path "$WORKDIR\conf") {
        $configLines += "releem_cnf_dir=`"$WORKDIR\conf`""
    }
    
    if ($mysqlLogin -and $mysqlPassword) {
        $configLines += "mysql_user=`"$mysqlLogin`""
        $configLines += "mysql_password=`"$mysqlPassword`""
    }
    
    if ($hostToUse -ne "127.0.0.1") {
        $configLines += "mysql_host=`"$hostToUse`""
    }
    
    if ($portToUse -ne "3306") {
        $configLines += "mysql_port=`"$portToUse`""
    }
    
    if ($mysqlLimit) {
        $configLines += "memory_limit=`"$mysqlLimit`""
    }
    
    # Windows service restart command
    $configLines += "mysql_restart_service=`"net stop mysql && net start mysql`""
    
    if (Test-Path $MYSQL_CONF_DIR) {
        $configLines += "mysql_cnf_dir=`"$MYSQL_CONF_DIR`""
    }
    
    $hostname = $env:RELEEM_HOSTNAME
    if (-not $hostname) {
        $hostname = $env:COMPUTERNAME
    }
    $configLines += "hostname=`"$hostname`""
    
    if ($env:RELEEM_ENV) {
        $configLines += "env=`"$($env:RELEEM_ENV)`""
    }
    
    if ($env:RELEEM_DEBUG) {
        $configLines += "debug=$($env:RELEEM_DEBUG)"
    }
    
    if ($env:RELEEM_MYSQL_SSL_MODE) {
        $configLines += "mysql_ssl_mode=$($env:RELEEM_MYSQL_SSL_MODE)"
    }
    
    if ($env:RELEEM_QUERY_OPTIMIZATION) {
        $configLines += "query_optimization=$($env:RELEEM_QUERY_OPTIMIZATION)"
    }
    
    if ($env:RELEEM_DATABASES_QUERY_OPTIMIZATION) {
        $configLines += "databases_query_optimization=`"$($env:RELEEM_DATABASES_QUERY_OPTIMIZATION)`""
    }
    
    $regionToUse = $Region
    if (-not $regionToUse) {
        $regionToUse = $env:RELEEM_REGION
    }
    if ($regionToUse) {
        $configLines += "releem_region=`"$regionToUse`""
    }
    
    $configLines += "interval_seconds=60"
    $configLines += "interval_read_config_seconds=3600"
    
    $configLines | Out-File -FilePath $CONF -Encoding UTF8
    
    Write-Log "* Configuring scheduled task"
    if (-not $SkipCron) {
        try {
            $taskName = "Releem Agent Config Update"
            $taskCommand = "powershell.exe"
            $taskArguments = "-ExecutionPolicy Bypass -File `"$WORKDIR\mysqlconfigurer.ps1`" -u"
            
            $action = New-ScheduledTaskAction -Execute $taskCommand -Argument $taskArguments
            $trigger = New-ScheduledTaskTrigger -Daily -At "00:00"
            $settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries
            $principal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -LogonType ServiceAccount
            
            Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $trigger -Settings $settings -Principal $principal -Force
            Write-Log "Scheduled task configuration complete. Automatic updates are enabled."
        }
        catch {
            Write-Log "Scheduled task configuration failed. Automatic updates are disabled: $($_.Exception.Message)"
        }
    }
    
    if (-not $DisableAgent) {
        Write-Log "* Executing Releem Agent for the first time"
        Write-Log "This may take up to 15 minutes on servers with many databases."
        try {
            & "$WORKDIR\releem-agent.exe" -f
            Start-Sleep -Seconds 3
            & "$WORKDIR\releem-agent.exe"
        }
        catch {
            Write-Log "Warning: First run of Releem Agent encountered issues"
        }
    }
    
    Write-Log "* Installing and starting Releem Agent service to collect metrics"
    try {
        & "$WORKDIR\releem-agent.exe" remove 2>$null
        & "$WORKDIR\releem-agent.exe" install
        Write-Log "The Releem Agent installation successful."
    }
    catch {
        Write-Log "The Releem Agent installation failed: $($_.Exception.Message)"
    }
    
    try {
        & "$WORKDIR\releem-agent.exe" stop 2>$null
        & "$WORKDIR\releem-agent.exe" start
        Write-Log "The Releem Agent restart successful."
    }
    catch {
        Write-Log "The Releem Agent restart failed: $($_.Exception.Message)"
    }
    
    Start-Sleep -Seconds 3
    
    # Check if process is running
    if (-not (Get-Process -Name "releem-agent" -ErrorAction SilentlyContinue)) {
        Handle-Error "The releem-agent process was not found! Check the system log for an error."
    }
    
    # Enable performance schema
    try {
        & "$WORKDIR\mysqlconfigurer.ps1" -p
    }
    catch {
        Write-Log "Warning: Could not enable performance schema"
    }
    
    Write-Log ""
    Write-Log "* Releem Agent has been successfully installed."
    Write-Log ""
    Write-Log "* To view Releem recommendations and MySQL metrics, visit https://app.releem.com/dashboard"
    Write-Log ""
    
    Send-LogToAPI -ApiKey $script:ApiKey
}

# Initialize log file
"" | Out-File -FilePath $LOGFILE -Encoding UTF8

# Start installation
try {
    Install-ReleemAgent
}
catch {
    Handle-Error "Installation failed: $($_.Exception.Message)"
}
