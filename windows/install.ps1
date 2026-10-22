<#
.SYNOPSIS
    Releem Agent installer for Windows.

.DESCRIPTION
    Installs, updates, or uninstalls the Releem Agent on Windows.
    Can be deployed silently via environment variables or interactively.

.PARAMETER u
    Run in update mode: replace the agent binary and restart the service.

.PARAMETER Uninstall
    Run in uninstall mode: stop and remove the agent, service, and scheduled task.

.EXAMPLE
    # Install (default)
    .\install.ps1

    # Update
    .\install.ps1 -u

    # Uninstall
    .\install.ps1 -Uninstall
#>

[CmdletBinding()]
param(
    [switch]$u,
    [switch]$Uninstall
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

$AgentBinaryPath         = 'C:\Program Files\ReleemAgent\releem-agent.exe'
$InstallerScriptPath     = 'C:\Program Files\ReleemAgent\install.ps1'
$ConfigurerScriptPath    = 'C:\Program Files\ReleemAgent\releem.ps1'
$LegacyConfigurerScriptPath = 'C:\Program Files\ReleemAgent\mysqlconfigurer.ps1'
$ConfigFilePath          = 'C:\ProgramData\ReleemAgent\releem.conf'
$ConfDirPath             = 'C:\ProgramData\ReleemAgent\conf.d'
$InstallLogPath          = 'C:\ProgramData\ReleemAgent\releem-install.log'
$DownloadBaseUrl         = 'https://releem.s3.us-east-1.amazonaws.com/v2'
$ApiUrlUS                = 'https://api.releem.com'
$ApiUrlEU                = 'https://api.eu.releem.com'

# ---------------------------------------------------------------------------
# Helper: Write-Log
# Writes a message to both the console and the install log file.
# ---------------------------------------------------------------------------

function Write-Log {
    param([string]$Message)
    Write-Host $Message
    Add-Content -Path $InstallLogPath -Value $Message
}

function Install-AgentFiles {
    param([string]$StopReason)

    # Validate the complete set before replacing any installed file. In particular,
    # a failed script download must never leave a new agent starting with old scripts.
    $downloadDir = Join-Path ([System.IO.Path]::GetTempPath()) ([guid]::NewGuid().ToString())
    New-Item -Path $downloadDir -ItemType Directory -ErrorAction Stop | Out-Null
    $keepRecoveryFiles = $false
    try {
        $files = @(
            @{ Name = 'install.ps1'; Destination = $InstallerScriptPath },
            @{ Name = 'releem.ps1'; Destination = $ConfigurerScriptPath },
            @{ Name = 'mysqlconfigurer.ps1'; Destination = $LegacyConfigurerScriptPath },
            @{ Name = 'releem-agent.exe'; Destination = $AgentBinaryPath }
        )
        foreach ($file in $files) {
            $source = Join-Path $downloadDir $file.Name
            $uri = "$DownloadBaseUrl/$($file.Name)"
            Write-Log "Downloading agent file from $uri ..."
            Invoke-WebRequest -Uri $uri -OutFile $source -UseBasicParsing -ErrorAction Stop
            if (-not (Test-Path -LiteralPath $source -PathType Leaf) -or (Get-Item -LiteralPath $source).Length -le 0) {
                throw "Downloaded agent file is missing or empty: $uri"
            }
            if ($file.Name -like '*.ps1') {
                $tokens = $null; $parseErrors = $null
                $null = [System.Management.Automation.Language.Parser]::ParseFile($source, [ref]$tokens, [ref]$parseErrors)
                if ($parseErrors.Count -gt 0) { throw "Downloaded script contains PowerShell syntax errors: $uri" }
            }
        }
        $originalService = Get-Service -Name 'releem-agent' -ErrorAction SilentlyContinue
        $serviceWasRunning = $originalService -and $originalService.Status -eq 'Running'
        Stop-ReleemAgentForReplacement -Reason $StopReason
        # Save the entire original set before replacing anything. Missing files
        # must remain missing if a fresh install or legacy upgrade fails.
        $backupDir = Join-Path $downloadDir 'originals'
        New-Item -Path $backupDir -ItemType Directory -ErrorAction Stop | Out-Null
        foreach ($file in $files) {
            $file.HadOriginal = Test-Path -LiteralPath $file.Destination -PathType Leaf
            if ($file.HadOriginal) {
                Copy-Item -LiteralPath $file.Destination -Destination (Join-Path $backupDir $file.Name) -Force -ErrorAction Stop
            } elseif (Test-Path -LiteralPath $file.Destination) {
                throw "Cannot replace agent file: destination is not a file: $($file.Destination)"
            }
        }
        $attemptedFiles = @()
        try {
            foreach ($file in $files) {
                # A failed copy may already have truncated its destination.
                $attemptedFiles += $file
                Copy-Item -LiteralPath (Join-Path $downloadDir $file.Name) -Destination $file.Destination -Force -ErrorAction Stop
            }
        } catch {
            $replacementError = $_
            $restoreErrors = @()
            foreach ($file in $attemptedFiles) {
                try {
                    if ($file.HadOriginal) {
                        Copy-Item -LiteralPath (Join-Path $backupDir $file.Name) -Destination $file.Destination -Force -ErrorAction Stop
                    } elseif (Test-Path -LiteralPath $file.Destination) {
                        Remove-Item -LiteralPath $file.Destination -Force -ErrorAction Stop
                    }
                } catch {
                    $restoreErrors += "$($file.Destination): $_"
                }
            }
            if ($restoreErrors.Count -gt 0) {
                $keepRecoveryFiles = $true
                $absentPaths = @($files | Where-Object { -not $_.HadOriginal } | ForEach-Object { $_.Destination }) -join ', '
                throw "Agent file replacement failed: $replacementError. Automatic restore failed: $($restoreErrors -join '; '). Recovery backups retained at '$backupDir'. Restore the saved files to their original destinations before starting the agent. Remove any new files at originally absent destinations: $absentPaths"
            }
            $recoveryStatus = 'The original file set was restored.'
            if ($serviceWasRunning) {
                try {
                    Start-Service -Name 'releem-agent' -ErrorAction Stop
                    $restoredService = Get-Service -Name 'releem-agent' -ErrorAction Stop
                    if (-not $restoredService -or $restoredService.Status -ne 'Running') {
                        throw 'Releem Agent service is not running after the restart attempt.'
                    }
                } catch {
                    $keepRecoveryFiles = $true
                    throw "Agent file replacement failed: $replacementError. The original file set was restored, but the Releem Agent service restart failed: $_. Recovery backups retained at '$backupDir'. Check the Windows service logs, then run Start-Service -Name 'releem-agent' to restore service."
                }
                $recoveryStatus += ' The previously running Releem Agent service was restarted.'
            }
            throw "Agent file replacement failed: $replacementError. $recoveryStatus Retry the installation."
        }
    } finally {
        if (-not $keepRecoveryFiles) {
            Remove-Item -LiteralPath $downloadDir -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}

# ---------------------------------------------------------------------------
# Helper: Invoke-Icacls
# Resolves the built-in utility without relying on PATH or PATHEXT and fails
# with an actionable error when the utility is missing or rejects the ACL.
# ---------------------------------------------------------------------------

function Invoke-Icacls {
    param([Parameter(Mandatory = $true)][string[]]$Arguments)

    $windowsDirectory = [Environment]::GetFolderPath([Environment+SpecialFolder]::Windows)
    $icaclsExe = Join-Path $windowsDirectory 'System32\icacls.exe'

    if (-not (Test-Path -LiteralPath $icaclsExe -PathType Leaf)) {
        throw "Required Windows utility not found: $icaclsExe"
    }

    & $icaclsExe @Arguments | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "icacls.exe failed with exit code $LASTEXITCODE while processing '$($Arguments[0])'."
    }
}

# ---------------------------------------------------------------------------
# Helper: Find-MyIniPath
# Searches standard locations for my.ini; returns full path or $null.
# ---------------------------------------------------------------------------

function Find-MyIniPath {
    foreach ($pattern in @(
        'C:\ProgramData\MySQL\MySQL Server *\my.ini',
        'C:\Program Files\MySQL\MySQL Server *\my.ini',
        'C:\Program Files\MariaDB*\data\my.ini'
    )) {
        $found = Get-Item $pattern -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($found) { return $found.FullName }
    }
    return $null
}

# ---------------------------------------------------------------------------
# Helper: Invoke-MySQL
# Runs mysql.exe suppressing stderr warnings without triggering
# $ErrorActionPreference = 'Stop' on native command stderr output.
# Returns captured stdout; sets $LASTEXITCODE as usual.
# ---------------------------------------------------------------------------

function Invoke-MySQL {
    $prev = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    $output = & $MysqlExe @args 2>$null
    $ErrorActionPreference = $prev
    return $output
}

function Get-MySQLRootArgs {
    $rootUser = if ($env:RELEEM_MYSQL_ROOT_LOGIN) { $env:RELEEM_MYSQL_ROOT_LOGIN } else { 'root' }
    $rootArgs = @('-u', $rootUser)
    if ([System.Environment]::GetEnvironmentVariable('RELEEM_MYSQL_ROOT_PASSWORD') -ne $null) {
        $rootArgs += "-p$env:RELEEM_MYSQL_ROOT_PASSWORD"
    }
    return $rootArgs
}

function Invoke-MySQLRoot {
    $mysqlArgs = @('-h', $MysqlHost, '-P', $MysqlPort) + (Get-MySQLRootArgs) + $args
    return Invoke-MySQL @mysqlArgs
}

function Test-MySQLRootConnection {
    $null = Invoke-MySQLRoot -e 'SELECT 1;'
    return ($LASTEXITCODE -eq 0)
}

function Convert-SecureStringToPlainText {
    param([System.Security.SecureString]$SecureString)

    $bstr = [System.Runtime.InteropServices.Marshal]::SecureStringToBSTR($SecureString)
    try {
        return [System.Runtime.InteropServices.Marshal]::PtrToStringBSTR($bstr)
    } finally {
        if ($bstr -ne [IntPtr]::Zero) {
            [System.Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr)
        }
    }
}

function Prompt-MySQLRootPasswordUntilSuccess {
    while ($true) {
        if ([Console]::IsInputRedirected) {
            Write-Host 'Please enter MySQL root password'
            $env:RELEEM_MYSQL_ROOT_PASSWORD = [Console]::In.ReadLine()
        } else {
            $securePassword = Read-Host -Prompt 'Please enter MySQL root password' -AsSecureString
            $env:RELEEM_MYSQL_ROOT_PASSWORD = Convert-SecureStringToPlainText $securePassword
        }

        if (Test-MySQLRootConnection) {
            return $true
        }

        Write-Log 'ERROR: MySQL connection failed with the entered root password.'
    }
}

# ---------------------------------------------------------------------------
# Helper: Stop-ReleemAgentForReplacement
# Stops the running service and waits for the binary to be released so the
# installer can safely overwrite releem-agent.exe on reinstall/update.
# ---------------------------------------------------------------------------

function Stop-ReleemAgentForReplacement {
    param(
        [string]$Reason = 'before replacing releem-agent.exe'
    )

    $svc = Get-Service -Name 'releem-agent' -ErrorAction SilentlyContinue
    if (-not $svc) {
        Write-Log "No existing Releem Agent service found; skipping stop $Reason."
        return
    }

    Write-Log "Stopping Releem Agent service $Reason..."

    if (Test-Path $AgentBinaryPath) {
        & $AgentBinaryPath stop
        Write-Log "releem-agent.exe stop exited with code: $LASTEXITCODE"
    }

    $svc = Get-Service -Name 'releem-agent' -ErrorAction SilentlyContinue
    if ($svc -and $svc.Status -eq 'Running') {
        Stop-Service -Name 'releem-agent' -ErrorAction SilentlyContinue
    }

    $waited = 0
    while ($waited -lt 30) {
        $svc = Get-Service -Name 'releem-agent' -ErrorAction SilentlyContinue
        if (-not $svc -or $svc.Status -ne 'Running') {
            Write-Log 'Releem Agent service is stopped.'
            return
        }
        Start-Sleep -Seconds 1
        $waited++
    }

    throw "Releem Agent service did not stop within 30 seconds $Reason."
}

# ---------------------------------------------------------------------------
# Admin privilege check
# ---------------------------------------------------------------------------

$currentPrincipal = [Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
if (-not $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Host 'ERROR: This script must be run as Administrator.'
    exit 1
}

# ---------------------------------------------------------------------------
# Logging setup: create log directory and write timestamped run header
# ---------------------------------------------------------------------------

New-Item -Path 'C:\ProgramData\ReleemAgent' -ItemType Directory -Force | Out-Null
$timestamp = Get-Date -Format 'yyyy-MM-dd HH:mm:ss'
Add-Content -Path $InstallLogPath -Value "=== Releem Install $timestamp ==="

# ---------------------------------------------------------------------------
# Exit code tracker and log-upload helper
# ---------------------------------------------------------------------------

$script:MainExitCode = 0

function Send-InstallLog {
    $apiKeyVar = Get-Variable -Name 'ApiKey' -Scope Script -ErrorAction SilentlyContinue
    $apiKey = if ($apiKeyVar) { $apiKeyVar.Value } else { '' }

    if (-not $apiKey) {
        Write-Host 'INFO: No API key available; skipping install log upload.'
        return
    }

    $apiBase = if ($env:RELEEM_REGION -eq 'EU') { $ApiUrlEU } else { $ApiUrlUS }
    $logUrl = "$apiBase/v2/events/saving_log"

    try {
        $logContent = if (Test-Path $InstallLogPath) { Get-Content -Path $InstallLogPath -Raw } else { '' }
        $headers = @{ 'x-releem-api-key' = $apiKey }
        Invoke-RestMethod -Uri $logUrl -Method Post -Body $logContent -Headers $headers -ErrorAction Stop | Out-Null
        Write-Host 'Install log uploaded to Releem API.'
    } catch {
        $warnMsg = "WARNING: Failed to upload install log to Releem API: $_"
        try { Write-Log $warnMsg } catch { Write-Host $warnMsg }
    }
}

# ---------------------------------------------------------------------------
# Mode dispatch
# ---------------------------------------------------------------------------

try {

if ($u) {
    Write-Log '=== Update mode ==='

    Install-AgentFiles -StopReason 'before update'

    # ---------------------------------------------------------------------------
    # Start the service
    # ---------------------------------------------------------------------------
    Write-Log 'Starting Releem Agent service...'
    & $AgentBinaryPath start
    if ($LASTEXITCODE -ne 0) {
        Write-Log "ERROR: 'releem-agent.exe start' failed with exit code $LASTEXITCODE."
        $script:MainExitCode = 1; return
    }
    Write-Log 'Releem Agent service start command issued.'

    # Verify the service is actually running
    $svc = Get-Service -Name 'releem-agent' -ErrorAction SilentlyContinue
    if (-not $svc -or $svc.Status -ne 'Running') {
        $svcStatus = if ($svc) { $svc.Status } else { 'not found' }
        Write-Log "ERROR: Releem Agent service is not running after update. Current status: $svcStatus"
        $script:MainExitCode = 1; return
    }
    Write-Log 'Releem Agent service is running.'

    Write-Log 'Releem Agent updated successfully.'
    $script:MainExitCode = 0; return
}

if ($Uninstall) {
    Write-Log '=== Uninstall mode ==='

    # Read API key from env var or existing releem.conf for log upload
    if ($env:RELEEM_API_KEY) {
        $ApiKey = $env:RELEEM_API_KEY
    } elseif (Test-Path $ConfigFilePath) {
        $confLine = Get-Content $ConfigFilePath | Where-Object { $_ -match '^apikey=' }
        if ($confLine) { $ApiKey = ($confLine -replace '^apikey="?([^"]*)"?$', '$1') }
    }

    # ---------------------------------------------------------------------------
    # Confirmation prompt
    # ---------------------------------------------------------------------------
    if ($env:RELEEM_UNINSTALL_CONFIRM -eq '1') {
        Write-Log 'Uninstall confirmed via RELEEM_UNINSTALL_CONFIRM=1 (silent mode).'
    } else {
        $confirmChoice = Read-Host 'This will remove the Releem Agent. Continue? [Y/n]'
        if ($confirmChoice -eq 'n' -or $confirmChoice -eq 'N') {
            Write-Log 'Uninstall aborted by user.'
            $script:MainExitCode = 0; return
        }
    }

    # ---------------------------------------------------------------------------
    # Stop and uninstall the service (non-fatal if binary missing)
    # ---------------------------------------------------------------------------
    if (Test-Path $AgentBinaryPath) {
        Write-Log 'Stopping Releem Agent service...'
        & $AgentBinaryPath stop
        Write-Log "releem-agent.exe stop exited with code: $LASTEXITCODE"

        Write-Log 'Removing Releem Agent service...'
        & $AgentBinaryPath remove
        Write-Log "releem-agent.exe remove exited with code: $LASTEXITCODE"
    } else {
        Write-Log 'WARNING: releem-agent.exe not found; skipping service stop and remove.'
    }

    # ---------------------------------------------------------------------------
    # Remove Scheduled Task
    # ---------------------------------------------------------------------------
    Unregister-ScheduledTask -TaskName 'ReleemAgentUpdate' -Confirm:$false -ErrorAction SilentlyContinue
    Write-Log 'Scheduled Task ReleemAgentUpdate removed (if it existed).'

    # ---------------------------------------------------------------------------
    # Remove agent directories
    # ---------------------------------------------------------------------------
    $agentDir = 'C:\Program Files\ReleemAgent'
    if (Test-Path $agentDir) {
        Write-Log "Removing: $agentDir"
        Remove-Item -Path $agentDir -Recurse -Force
        Write-Host "Removed: $agentDir"
    } else {
        Write-Log "Directory not found (already removed?): $agentDir"
    }

    # ---------------------------------------------------------------------------
    # Remove !includedir from my.ini
    # ---------------------------------------------------------------------------
    $myIniPath = Find-MyIniPath
    if ($myIniPath) {
        $ansi = [System.Text.Encoding]::Default
        $myIniContent = [System.IO.File]::ReadAllText($myIniPath, $ansi)
        if ($myIniContent -match '!includedir[^\r\n]*releem\.conf\.d') {
            $newContent = $myIniContent -replace '\r?\n!includedir[^\r\n]*releem\.conf\.d[^\r\n]*', ''
            [System.IO.File]::WriteAllText($myIniPath, $newContent, $ansi)
            Write-Log "Removed !includedir from $myIniPath"
        } else {
            Write-Log "!includedir not found in $myIniPath - no changes made."
        }
    } else {
        Write-Log 'WARNING: my.ini not found - could not remove !includedir automatically.'
    }

    Write-Log 'Removing C:\ProgramData\ReleemAgent (including log file)...'

    $dataDir = 'C:\ProgramData\ReleemAgent'
    if (Test-Path $dataDir) {
        # Reset ACLs on all files before removal (releem.conf has restricted permissions)
        Invoke-Icacls -Arguments @($dataDir, '/reset', '/T', '/Q')
        Remove-Item -Path $dataDir -Recurse -Force
        Write-Host "Removed: $dataDir"
    } else {
        Write-Host "Directory not found (already removed?): $dataDir"
    }

    Write-Host 'Releem Agent uninstalled successfully.'
    $script:MainExitCode = 0; return
}

# Default: Install mode
Write-Log 'Install mode'

# ---------------------------------------------------------------------------
# API key collection
# ---------------------------------------------------------------------------

if ($env:RELEEM_API_KEY) {
    $ApiKey = $env:RELEEM_API_KEY
} else {
    $ApiKey = Read-Host 'Enter your Releem API key'
}

if (-not $ApiKey) {
    Write-Log 'ERROR: Releem API key is required.'
    $script:MainExitCode = 1; return
}

# ---------------------------------------------------------------------------
# MySQL connection setup
# ---------------------------------------------------------------------------

$MysqlHost = if ($env:RELEEM_MYSQL_HOST) { $env:RELEEM_MYSQL_HOST } else { '127.0.0.1' }
$MysqlPort = if ($env:RELEEM_MYSQL_PORT) { $env:RELEEM_MYSQL_PORT } else { '3306' }

# Locate mysql.exe: search PATH first, then well-known install locations
$MysqlExe = $null
$mysqlCmd = Get-Command mysql.exe -ErrorAction SilentlyContinue
if ($mysqlCmd) {
    $MysqlExe = $mysqlCmd.Source
} else {
    foreach ($pattern in @('C:\Program Files\MySQL\*\bin\mysql.exe', 'C:\Program Files\MariaDB*\bin\mysql.exe')) {
        $found = Get-Item $pattern -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($found) { $MysqlExe = $found.FullName; break }
    }
}

if (-not $MysqlExe) {
    Write-Log 'ERROR: mysql.exe not found. Please install MySQL client tools and ensure mysql.exe is in your PATH.'
    $script:MainExitCode = 1; return
}

Write-Log "Using MySQL client: $MysqlExe"

# ---------------------------------------------------------------------------
# Required directory creation
# ---------------------------------------------------------------------------

Write-Log 'Creating required directories...'

New-Item -Path 'C:\ProgramData\ReleemAgent' -ItemType Directory -Force | Out-Null
Write-Log 'Directory ready: C:\ProgramData\ReleemAgent'

New-Item -Path 'C:\ProgramData\ReleemAgent\conf.d' -ItemType Directory -Force | Out-Null
Write-Log 'Directory ready: C:\ProgramData\ReleemAgent\conf.d'

New-Item -Path 'C:\Program Files\ReleemAgent' -ItemType Directory -Force | Out-Null
Write-Log 'Directory ready: C:\Program Files\ReleemAgent'

# Download and validate all required files before stopping the existing agent.
Install-AgentFiles -StopReason 'before reinstall'

# ---------------------------------------------------------------------------
# MySQL user setup (create_mysql_user)
# ---------------------------------------------------------------------------

$InstanceType        = if ($env:RELEEM_INSTANCE_TYPE) { $env:RELEEM_INSTANCE_TYPE } else { 'local' }
$ReleemMysqlLogin    = ''
$ReleemMysqlPassword = ''
$at                  = '@'
$MysqlUserHost       = if ($MysqlHost -eq '127.0.0.1' -or $MysqlHost -eq 'localhost') { '127.0.0.1' } else { '%' }

Write-Log 'Configuring the MySQL user for metrics collection.'

$FLAG_SUCCESS = 0

if ($env:RELEEM_MYSQL_PASSWORD -and $env:RELEEM_MYSQL_LOGIN) {
    Write-Log 'Using MySQL login and password from environment variables.'
    $ReleemMysqlLogin    = $env:RELEEM_MYSQL_LOGIN
    $ReleemMysqlPassword = $env:RELEEM_MYSQL_PASSWORD
    $FLAG_SUCCESS = 1

} else {
    $MysqlRootLogin = if ($env:RELEEM_MYSQL_ROOT_LOGIN) { $env:RELEEM_MYSQL_ROOT_LOGIN } else { 'root' }
    Write-Log "Using MySQL root user '$MysqlRootLogin'."

    $rootConnectionSuccessful = Test-MySQLRootConnection
    if (-not $rootConnectionSuccessful -and [System.Environment]::GetEnvironmentVariable('RELEEM_MYSQL_ROOT_PASSWORD') -eq $null) {
        $rootConnectionSuccessful = Prompt-MySQLRootPasswordUntilSuccess
    }

    if ($rootConnectionSuccessful) {
        Write-Log 'MySQL connection successful.'

        $ReleemMysqlLogin    = 'releem'
        $charSet             = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!#%'.ToCharArray()
        $ReleemMysqlPassword = -join (1..16 | ForEach-Object { $charSet | Get-Random })

        $null = Invoke-MySQLRoot -e "DROP USER '$ReleemMysqlLogin'$at'$MysqlUserHost';"
        $null = Invoke-MySQLRoot -e "CREATE USER '$ReleemMysqlLogin'$at'$MysqlUserHost' IDENTIFIED BY '$ReleemMysqlPassword';"
        $null = Invoke-MySQLRoot -e "GRANT PROCESS ON *.* TO '$ReleemMysqlLogin'$at'$MysqlUserHost';"
        $null = Invoke-MySQLRoot -e "GRANT REPLICATION CLIENT ON *.* TO '$ReleemMysqlLogin'$at'$MysqlUserHost';"
        $null = Invoke-MySQLRoot -e "GRANT SHOW VIEW ON *.* TO '$ReleemMysqlLogin'$at'$MysqlUserHost';"
        $null = Invoke-MySQLRoot -e "GRANT SELECT ON mysql.* TO '$ReleemMysqlLogin'$at'$MysqlUserHost';"

        # Non-fatal performance_schema grants
        $null = Invoke-MySQLRoot -e "GRANT SELECT ON performance_schema.events_statements_summary_by_digest TO '$ReleemMysqlLogin'$at'$MysqlUserHost';"
        $null = Invoke-MySQLRoot -e "GRANT SELECT ON performance_schema.table_io_waits_summary_by_index_usage TO '$ReleemMysqlLogin'$at'$MysqlUserHost';"
        $null = Invoke-MySQLRoot -e "GRANT SELECT ON performance_schema.file_summary_by_instance TO '$ReleemMysqlLogin'$at'$MysqlUserHost';"

        # SYSTEM_VARIABLES_ADMIN or SUPER (non-fatal)
        $null = Invoke-MySQLRoot -e "GRANT SYSTEM_VARIABLES_ADMIN ON *.* TO '$ReleemMysqlLogin'$at'$MysqlUserHost';"
        if ($LASTEXITCODE -ne 0) {
            $null = Invoke-MySQLRoot -e "GRANT SUPER ON *.* TO '$ReleemMysqlLogin'$at'$MysqlUserHost';"
        }

        if ($env:RELEEM_QUERY_OPTIMIZATION) {
            $null = Invoke-MySQLRoot -e "GRANT SELECT ON *.* TO '$ReleemMysqlLogin'$at'$MysqlUserHost';"
        }

        Write-Log "Created new user '$ReleemMysqlLogin'."
        $FLAG_SUCCESS = 1

    } else {
        Write-Log "ERROR: MySQL connection failed with user '$MysqlRootLogin'. Check that RELEEM_MYSQL_ROOT_LOGIN and RELEEM_MYSQL_ROOT_PASSWORD are correct and reinstall the agent."
        $script:MainExitCode = 1; return
    }
}

if ($FLAG_SUCCESS -eq 1) {
    $null = Invoke-MySQL -h $MysqlHost -P $MysqlPort -u $ReleemMysqlLogin "-p$ReleemMysqlPassword" -e 'SELECT 1;'
    if ($LASTEXITCODE -eq 0) {
        Write-Log "MySQL connection with user '$ReleemMysqlLogin' - successful."
    } else {
        Write-Log "ERROR: MySQL connection failed with user '$ReleemMysqlLogin'. Check that the user and password is correct and reinstall the agent."
        $script:MainExitCode = 1; return
    }
}

# ---------------------------------------------------------------------------
# MySQL conf dir detection (used in releem.conf; -Configure writes the files)
# ---------------------------------------------------------------------------

$MysqlConfDir = ''

if ($InstanceType -ne 'aws/rds' -and $InstanceType -ne 'gcp/cloudsql') {
    $myIniPath = Find-MyIniPath
    if ($myIniPath) {
        Write-Log "Found MySQL config file: $myIniPath"
        $MysqlConfDir = Join-Path (Split-Path $myIniPath -Parent) 'releem.conf.d'
    } else {
        Write-Log 'WARNING: MySQL config file (my.ini) not found in common locations.'
    }
}

# ---------------------------------------------------------------------------
# MySQL service detection
# ---------------------------------------------------------------------------

$MysqlServiceName = $null
if ($InstanceType -ne 'aws/rds' -and $InstanceType -ne 'gcp/cloudsql') {
    foreach ($name in @('MySQL267', 'MySQL97', 'MySQL90', 'MySQL84', 'MySQL80', 'MySQL57', 'MySQL56', 'MySQL', 'mariadb', 'mysqld')) {
        $svc = Get-Service -Name $name -ErrorAction SilentlyContinue
        if ($svc) {
            $MysqlServiceName = $name
            Write-Log "Detected MySQL service: $MysqlServiceName"
            break
        }
    }
    if (-not $MysqlServiceName) {
        Write-Log 'WARNING: Could not detect MySQL service name. mysql_restart_service will not be set in releem.conf.'
    }
}

# ---------------------------------------------------------------------------
# releem.conf generation with restricted ACL
# ---------------------------------------------------------------------------

$restartCmd = if ($MysqlServiceName) { "net stop $MysqlServiceName && net start $MysqlServiceName" } else { '' }
$confDirEscaped = $ConfDirPath -replace '\\', '\\'
$confLines = @(
    "apikey=`"$ApiKey`"",
    "releem_cnf_dir=`"$confDirEscaped`"",
    "mysql_host=`"$MysqlHost`"",
    "mysql_port=`"$MysqlPort`"",
    "mysql_user=`"$ReleemMysqlLogin`"",
    "mysql_password=`"$ReleemMysqlPassword`"",
    "mysql_restart_service=`"$restartCmd`"",
    "mysql_cnf_dir=`"$($MysqlConfDir -replace '\\', '\\')`"",
    'interval_seconds=60',
    'interval_read_config_seconds=3600',
    "hostname=`"$env:COMPUTERNAME`"",
    "instance_type=`"$InstanceType`""
)
$memoryLimit = if ($env:RELEEM_MYSQL_MEMORY_LIMIT) { $env:RELEEM_MYSQL_MEMORY_LIMIT } elseif ($env:RELEEM_DB_MEMORY_LIMIT) { $env:RELEEM_DB_MEMORY_LIMIT } else { $null }
if ($null -ne $memoryLimit) {
    $confLines += "memory_limit=$memoryLimit"
}
if ($env:RELEEM_QUERY_OPTIMIZATION) {
    $confLines += "query_optimization=$($env:RELEEM_QUERY_OPTIMIZATION)"
}
$utf8NoBom = New-Object System.Text.UTF8Encoding $false
[System.IO.File]::WriteAllText($ConfigFilePath, ($confLines -join "`r`n"), $utf8NoBom)
Write-Log "Created releem.conf: $ConfigFilePath"

# Restrict ACL: readable only by SYSTEM and Administrators
Invoke-Icacls -Arguments @(
    $ConfigFilePath,
    '/inheritance:r',
    '/grant:r', '*S-1-5-18:(R)',
    '/grant:r', '*S-1-5-32-544:(M)'
)
Write-Log 'releem.conf permissions restricted to SYSTEM and Administrators.'

# ---------------------------------------------------------------------------
# Windows Service installation and start
# ---------------------------------------------------------------------------

# Optional first-run flag check (skipped when RELEEM_AGENT_DISABLE=1)
if ($env:RELEEM_AGENT_DISABLE -ne '1') {
    Write-Log 'Running releem-agent.exe -f (initial flag check)...'
    & $AgentBinaryPath -f
    Write-Log "releem-agent.exe -f exited with code: $LASTEXITCODE"
}

$existingSvc = Get-Service -Name 'releem-agent' -ErrorAction SilentlyContinue
if ($existingSvc) {
    Write-Log 'Releem Agent service already exists - stopping and removing...'
    & $AgentBinaryPath stop
    & $AgentBinaryPath remove
    # Wait for SCM to fully remove the service (up to 10 seconds)
    $waited = 0
    while ((Get-Service -Name 'releem-agent' -ErrorAction SilentlyContinue) -and $waited -lt 10) {
        Start-Sleep -Seconds 1
        $waited++
    }
}

Write-Log 'Installing Releem Agent Windows Service...'
& $AgentBinaryPath install
if ($LASTEXITCODE -ne 0) {
    Write-Log "ERROR: 'releem-agent.exe install' failed with exit code $LASTEXITCODE."
    $script:MainExitCode = 1; return
}
Write-Log 'Releem Agent service installed.'

Write-Log 'Starting Releem Agent Windows Service...'
& $AgentBinaryPath start
if ($LASTEXITCODE -ne 0) {
    Write-Log "ERROR: 'releem-agent.exe start' failed with exit code $LASTEXITCODE."
    $script:MainExitCode = 1; return
}
Write-Log 'Releem Agent service start command issued.'

# Verify the service is actually running
$svc = Get-Service -Name 'releem-agent' -ErrorAction SilentlyContinue
if (-not $svc -or $svc.Status -ne 'Running') {
    $svcStatus = if ($svc) { $svc.Status } else { 'not found' }
    Write-Log "ERROR: Releem Agent service is not running. Current status: $svcStatus"
    $script:MainExitCode = 1; return
}
Write-Log "Releem Agent service is running."

# ---------------------------------------------------------------------------
# Scheduled Task for daily auto-updates
# ---------------------------------------------------------------------------

# ---------------------------------------------------------------------------
# Enable query monitoring for local instances (equivalent to Linux: $RELEEM_COMMAND -p)
# ---------------------------------------------------------------------------

if ($InstanceType -eq 'local') {
    if (Test-Path $ConfigurerScriptPath) {
        Write-Log 'Enabling query monitoring for local instance...'
        $powerShellExe = Join-Path ([Environment]::GetFolderPath([Environment+SpecialFolder]::Windows)) 'System32\WindowsPowerShell\v1.0\powershell.exe'
        if (-not (Test-Path -LiteralPath $powerShellExe -PathType Leaf)) {
            throw "Required Windows PowerShell executable not found: $powerShellExe"
        }
        & $powerShellExe -NonInteractive -File $ConfigurerScriptPath -Configure
        Write-Log "releem.ps1 -Configure exited with code: $LASTEXITCODE"
    } else {
        Write-Log 'WARNING: releem.ps1 not found; skipping -Configure.'
    }
}

# Determine whether to create the scheduled task
$createTask = $false
if ($env:RELEEM_CRON_ENABLE -eq '1') {
    $createTask = $true
    Write-Log 'Scheduled Task auto-update: enabled via RELEEM_CRON_ENABLE=1.'
} elseif ($env:RELEEM_CRON_ENABLE -eq '0') {
    Write-Log 'Scheduled Task auto-update: disabled via RELEEM_CRON_ENABLE=0. Skipping.'
} else {
    $cronChoice = Read-Host 'Enable daily auto-updates? [Y/n]'
    if ($cronChoice -eq '' -or $cronChoice -eq 'Y' -or $cronChoice -eq 'y') {
        $createTask = $true
    } else {
        Write-Log 'Scheduled Task auto-update: skipped by user.'
    }
}

if ($createTask) {
    $existingTask = Get-ScheduledTask -TaskName 'ReleemAgentUpdate' -ErrorAction SilentlyContinue
    if ($existingTask) {
        Write-Log 'Scheduled Task ReleemAgentUpdate already exists. Skipping registration.'
    } else {
        $taskAction    = New-ScheduledTaskAction -Execute 'powershell.exe' -Argument "-NonInteractive -File `"$InstallerScriptPath`" -u"
        $taskTrigger   = New-ScheduledTaskTrigger -Daily -At '00:00'
        $taskPrincipal = New-ScheduledTaskPrincipal -UserId 'SYSTEM' -LogonType ServiceAccount
        $taskSettings  = New-ScheduledTaskSettingsSet
        Register-ScheduledTask -TaskName 'ReleemAgentUpdate' -Action $taskAction -Trigger $taskTrigger -Principal $taskPrincipal -Settings $taskSettings | Out-Null
        Write-Log 'Scheduled Task ReleemAgentUpdate created: runs daily at 00:00 as SYSTEM.'
    }
}

Write-Log 'Releem Agent successfully installed. Check your dashboard at https://app.releem.com'
$script:MainExitCode = 0

} catch {
    Write-Log "ERROR: An unexpected error occurred: $_"
    Write-Log "ERROR: Stack trace: $($_.ScriptStackTrace)"
    $script:MainExitCode = 1
} finally {
    Send-InstallLog
    exit $script:MainExitCode
}
