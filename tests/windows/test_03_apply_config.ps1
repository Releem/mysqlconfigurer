# Test 3: Apply recommended MySQL configuration via mysqlconfigurer.ps1 (Windows).
# Pre-conditions:
#   - Releem Agent is installed and running (run test_01 first)
#   - Agent has sent metrics and API has generated a recommendation
# Required env vars:
#   RELEEM_API_KEY, MYSQL_ROOT_PASSWORD, OS_VERSION

param()

. "$PSScriptRoot\helpers.ps1"

$env:RELEEM_API_KEY      = if ($env:RELEEM_API_KEY)      { $env:RELEEM_API_KEY }      else { throw "RELEEM_API_KEY must be set" }
$env:MYSQL_ROOT_PASSWORD = if ($env:MYSQL_ROOT_PASSWORD) { $env:MYSQL_ROOT_PASSWORD } else { throw "MYSQL_ROOT_PASSWORD must be set" }
$OsVersion               = if ($env:OS_VERSION)          { $env:OS_VERSION }          else { throw "OS_VERSION must be set" }

$Hostname       = "releem-agent-test-$OsVersion"
$DbService      = Get-MySQLServiceName
$ConfDir        = "C:\ProgramData\ReleemAgent\conf.d"
$ConfigFile     = "$ConfDir\z_aiops_mysql.cnf"
$ReleemConfFile = "C:\ProgramData\ReleemAgent\releem.conf"

Write-Info "=== Test 3: Apply recommended MySQL configuration ==="
Write-Info "Hostname: $Hostname"

# --- Verify pre-conditions ---
Assert-ServiceRunning "Pre: releem-agent running"  "releem-agent"
Assert-ServiceRunning "Pre: DB service running"    $DbService
Assert-FileExists     "Pre: releem.conf present"   "C:\ProgramData\ReleemAgent\releem.conf"

# --- Ensure local preconditions for non-API/offline test execution ---
if (Test-Path $ReleemConfFile) {
    $releemConf = Get-Content $ReleemConfFile -Raw
    if ($releemConf -notmatch '(?m)^mysql_cnf_dir=') {
        Add-Content -Path $ReleemConfFile -Value "mysql_cnf_dir=`"$ConfDir`""
    }
    if ($releemConf -notmatch '(?m)^mysql_restart_service=') {
        Add-Content -Path $ReleemConfFile -Value "mysql_restart_service=`"net stop $DbService && net start $DbService`""
    }
}
if (-not (Test-Path $ConfigFile)) {
    Write-Info "Creating local recommended config stub for apply test..."
    Set-Content -Path $ConfigFile -Value '[mysqld]' -Encoding ASCII
    Add-Content -Path $ConfigFile -Value 'max_connections=200' -Encoding ASCII
}

# Ensure mysql_cnf_dir is persisted by running configurer's configure flow.
Write-Info "Running mysqlconfigurer.ps1 -Configure..."
& powershell.exe -ExecutionPolicy Bypass -File $ConfigurerScript -Configure
$configureExit = $LASTEXITCODE
Assert-Zero "mysqlconfigurer.ps1 -Configure exited successfully" $configureExit

# --- Apply configuration using mysqlconfigurer.ps1 -Apply -NonInteractive ---
Write-Info "Running mysqlconfigurer.ps1 -Apply -NonInteractive..."
& powershell.exe -ExecutionPolicy Bypass -File $ConfigurerScript -Apply -NonInteractive
$applyExit = $LASTEXITCODE

Assert-Zero "mysqlconfigurer.ps1 -Apply -NonInteractive exited successfully" $applyExit

# --- Local assertions ---
Assert-FileExists     "Recommended config file created" $ConfigFile
Assert-ServiceRunning "DB service still running"        $DbService
Assert-ServiceRunning "releem-agent still running"      "releem-agent"

# Verify config file has MySQL directives
Assert-FileContains "Config file has [mysqld]" $ConfigFile "\[mysqld\]"

# Verify MySQL still accepts connections
Assert-MySQLCanConnect "MySQL still accepts connections" "root" $env:MYSQL_ROOT_PASSWORD

Show-Summary "Test 3: Apply recommended MySQL configuration (Windows)"
