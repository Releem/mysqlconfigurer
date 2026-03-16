# Test 2: Releem Agent installation when a releem MySQL user already exists (Windows).
# Required env vars:
#   RELEEM_API_KEY, MYSQL_ROOT_PASSWORD, OS_VERSION
# Optional:
#   RELEEM_EXISTING_USER_PASSWORD

param()

. "$PSScriptRoot\helpers.ps1"

$env:RELEEM_API_KEY      = if ($env:RELEEM_API_KEY)      { $env:RELEEM_API_KEY }      else { throw "RELEEM_API_KEY must be set" }
$env:MYSQL_ROOT_PASSWORD = if ($env:MYSQL_ROOT_PASSWORD) { $env:MYSQL_ROOT_PASSWORD } else { throw "MYSQL_ROOT_PASSWORD must be set" }
$OsVersion               = if ($env:OS_VERSION)          { $env:OS_VERSION }          else { throw "OS_VERSION must be set" }

$Hostname        = "releem-agent-test-$OsVersion"
$ReleemUserPass  = if ($env:RELEEM_EXISTING_USER_PASSWORD) { $env:RELEEM_EXISTING_USER_PASSWORD } else { "ReleemTest!$(Get-Date -UFormat %s)" }
$DbService       = Get-MySQLServiceName

Write-Info "=== Test 2: Installation with pre-existing releem MySQL user ==="
Write-Info "Hostname: $Hostname"

# --- Pre-test cleanup ---
Remove-ReleemAgent
Remove-ReleemMySQLUser

# --- Create releem MySQL user manually ---
Write-Info "Creating releem MySQL user manually..."
$sql = @"
CREATE USER 'releem'@'127.0.0.1' IDENTIFIED BY '$ReleemUserPass';
GRANT PROCESS, REPLICATION CLIENT, SHOW VIEW ON *.* TO 'releem'@'127.0.0.1';
GRANT SELECT ON mysql.* TO 'releem'@'127.0.0.1';
GRANT SELECT ON performance_schema.events_statements_summary_by_digest TO 'releem'@'127.0.0.1';
GRANT SELECT ON performance_schema.table_io_waits_summary_by_index_usage TO 'releem'@'127.0.0.1';
GRANT SELECT ON performance_schema.file_summary_by_instance TO 'releem'@'127.0.0.1';
FLUSH PRIVILEGES;
"@
$sql | & mysql -u root -p"$env:MYSQL_ROOT_PASSWORD" 2>$null

Assert-MySQLUserExists "Pre: releem user pre-created" "releem"
Assert-MySQLCanConnect "Pre: releem user can connect" "releem" $ReleemUserPass

# --- Run install.ps1 with existing user credentials ---
Write-Info "Running install.ps1 with pre-existing user credentials..."
$env:RELEEM_HOSTNAME       = $Hostname
$env:RELEEM_MYSQL_LOGIN    = "releem"
$env:RELEEM_MYSQL_PASSWORD = $ReleemUserPass
$env:RELEEM_CRON_ENABLE    = "1"
$env:RELEEM_DB_MEMORY_LIMIT = "0"
# Do NOT set RELEEM_MYSQL_ROOT_PASSWORD

& powershell.exe -ExecutionPolicy Bypass -File $InstallScript
$installExit = $LASTEXITCODE

Assert-Zero "install.ps1 exited successfully" $installExit

# --- Local assertions ---
Assert-FileExists  "releem.conf created"         "C:\ProgramData\ReleemAgent\releem.conf"
Assert-DirExists   "releem conf.d dir created"   "C:\ProgramData\ReleemAgent\conf.d"
Assert-FileExists  "releem-agent.exe present"    "C:\Program Files\ReleemAgent\releem-agent.exe"
Assert-ServiceRunning "releem-agent service running" "releem-agent"

if (Test-Path "C:\ProgramData\ReleemAgent\releem.conf") {
    $confText = Get-Content "C:\ProgramData\ReleemAgent\releem.conf" -Raw
    if ($confText -match [regex]::Escape($Hostname) -or $confText -match [regex]::Escape($env:COMPUTERNAME)) {
        Write-Pass "releem.conf has hostname"
    } else {
        Write-Fail "releem.conf has hostname: neither '$Hostname' nor '$($env:COMPUTERNAME)' found"
    }
} else {
    Write-Fail "releem.conf has hostname: config file missing"
}
Assert-FileContains "releem.conf has api key"   "C:\ProgramData\ReleemAgent\releem.conf" $env:RELEEM_API_KEY
Assert-FileContains "releem.conf has mysql login" "C:\ProgramData\ReleemAgent\releem.conf" "releem"

# Verify existing user credentials were NOT changed
Assert-MySQLCanConnect "Existing user credentials still work" "releem" $ReleemUserPass

Show-Summary "Test 2: Install with pre-existing releem user (Windows)"
