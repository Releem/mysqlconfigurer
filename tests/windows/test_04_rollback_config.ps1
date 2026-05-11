# Test 4: Rollback previously applied MySQL configuration via mysqlconfigurer.ps1 -Rollback (Windows).
# Pre-conditions:
#   - Releem Agent is installed (run test_01 first)
#   - Configuration was already applied (run test_03 first)
# Required env vars:
#   RELEEM_API_KEY, MYSQL_ROOT_PASSWORD, OS_VERSION

param()

. "$PSScriptRoot\helpers.ps1"

$env:RELEEM_API_KEY      = if ($env:RELEEM_API_KEY)      { $env:RELEEM_API_KEY }      else { throw "RELEEM_API_KEY must be set" }
$env:MYSQL_ROOT_PASSWORD = if ($env:MYSQL_ROOT_PASSWORD) { $env:MYSQL_ROOT_PASSWORD } else { throw "MYSQL_ROOT_PASSWORD must be set" }
$OsVersion               = if ($env:OS_VERSION)          { $env:OS_VERSION }          else { throw "OS_VERSION must be set" }

$Hostname   = "releem-agent-test-$OsVersion"
$DbService  = Get-MySQLServiceName
$ConfDir    = "C:\ProgramData\ReleemAgent\conf.d"
$ConfigFile = "$ConfDir\z_aiops_mysql.cnf"
$ReleemConfFile = "C:\ProgramData\ReleemAgent\releem.conf"

Write-Info "=== Test 4: Rollback applied MySQL configuration ==="
Write-Info "Hostname: $Hostname"

# Ensure configurer metadata exists for deterministic rollback path.
if (Test-Path $ReleemConfFile) {
    $releemConf = Get-Content $ReleemConfFile -Raw
    if ($releemConf -notmatch '(?m)^mysql_cnf_dir=') {
        Add-Content -Path $ReleemConfFile -Value "mysql_cnf_dir=`"$ConfDir`""
    }
    if ($releemConf -notmatch '(?m)^mysql_restart_service=') {
        Add-Content -Path $ReleemConfFile -Value "mysql_restart_service=`"net stop $DbService && net start $DbService`""
    }
}

# --- Verify pre-conditions ---
Assert-ServiceRunning "Pre: releem-agent running" "releem-agent"
Assert-ServiceRunning "Pre: DB service running"   $DbService
Assert-FileExists     "Pre: config file present"  $ConfigFile

# --- Run rollback ---
Write-Info "Running mysqlconfigurer.ps1 -Rollback..."
$env:RELEEM_ROLLBACK_CONFIRM = "1"
& powershell.exe -ExecutionPolicy Bypass -File $ConfigurerScript -Rollback
$rollbackExit = $LASTEXITCODE
Remove-Item Env:RELEEM_ROLLBACK_CONFIRM -ErrorAction SilentlyContinue

Assert-Zero "mysqlconfigurer.ps1 -Rollback exited successfully" $rollbackExit

# --- Local assertions ---
Assert-ServiceRunning "DB service running after rollback"  $DbService
Assert-ServiceRunning "releem-agent still running"         "releem-agent"

if (-not (Test-Path $ConfigFile)) {
    Write-Pass "Rollback: config file removed"
} else {
    Write-Info "Config file still present after rollback (may contain restored backup content)"
}

# MySQL must still accept connections after restart
Assert-MySQLCanConnect "MySQL accepts connections after rollback" "root" $env:MYSQL_ROOT_PASSWORD

Show-Summary "Test 4: Rollback applied MySQL configuration (Windows)"
