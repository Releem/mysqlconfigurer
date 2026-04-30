# Test 1: Fresh Releem Agent installation with automatic releem MySQL user creation (Windows).
# Pre-conditions:
#   - MySQL/MariaDB is running with world DB loaded
#   - releem MySQL user does NOT exist
#   - No previous Releem installation
# Required env vars:
#   RELEEM_API_KEY, MYSQL_ROOT_PASSWORD, OS_VERSION

param()

. "$PSScriptRoot\helpers.ps1"

$env:RELEEM_API_KEY    = if ($env:RELEEM_API_KEY)    { $env:RELEEM_API_KEY }    else { throw "RELEEM_API_KEY must be set" }
$env:MYSQL_ROOT_PASSWORD = if ($env:MYSQL_ROOT_PASSWORD) { $env:MYSQL_ROOT_PASSWORD } else { throw "MYSQL_ROOT_PASSWORD must be set" }
$OsVersion = if ($env:OS_VERSION) { $env:OS_VERSION } else { throw "OS_VERSION must be set" }

$Hostname  = "releem-agent-test-$OsVersion"
$DbService = Get-MySQLServiceName

Write-Info "=== Test 1: Fresh installation with automatic releem user creation ==="
Write-Info "Hostname: $Hostname"

# --- Pre-test cleanup ---
Remove-ReleemAgent
Remove-ReleemMySQLUser

# --- Verify pre-conditions ---
Assert-ServiceRunning "Pre: DB service running" $DbService

$count = & mysql -u root -p"$env:MYSQL_ROOT_PASSWORD" -sNe "SELECT COUNT(*) FROM mysql.user WHERE User='releem';" 2>$null
if ([int]$count -gt 0) {
    Write-Fail "Pre: releem MySQL user should not exist before test"
    exit 1
}
Write-Info "Pre-conditions OK: DB running, no releem user"

# --- Run install.ps1 ---
Write-Info "Running install.ps1..."
$env:RELEEM_HOSTNAME           = $Hostname
$env:RELEEM_MYSQL_ROOT_PASSWORD = $env:MYSQL_ROOT_PASSWORD
$env:RELEEM_CRON_ENABLE        = "1"
$env:RELEEM_DB_MEMORY_LIMIT    = "0"

& powershell.exe -ExecutionPolicy Bypass -File $InstallScript
$installExit = $LASTEXITCODE

Assert-Zero "install.ps1 exited successfully" $installExit

# --- Local assertions ---
Assert-FileExists  "releem.conf created"          "C:\ProgramData\ReleemAgent\releem.conf"
Assert-DirExists   "releem conf.d dir created"    "C:\ProgramData\ReleemAgent\conf.d"
Assert-FileExists  "releem-agent.exe present"     "C:\Program Files\ReleemAgent\releem-agent.exe"
Assert-ServiceRunning "releem-agent service running" "releem-agent"
Assert-ScheduledTaskExists "ReleemAgentUpdate task created" "ReleemAgentUpdate"

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

Assert-MySQLUserExists "releem MySQL user created" "releem"

Show-Summary "Test 1: Fresh install with auto user creation (Windows)"
