# Test 9: Re-running install.ps1 should rewrite releem.conf without prompting and keep credentials consistent.

param()

. "$PSScriptRoot\helpers.ps1"

$env:RELEEM_API_KEY      = if ($env:RELEEM_API_KEY) { $env:RELEEM_API_KEY } else { throw "RELEEM_API_KEY must be set" }
$env:MYSQL_ROOT_PASSWORD = if ($env:MYSQL_ROOT_PASSWORD) { $env:MYSQL_ROOT_PASSWORD } else { throw "MYSQL_ROOT_PASSWORD must be set" }
$OsVersion               = if ($env:OS_VERSION) { $env:OS_VERSION } else { throw "OS_VERSION must be set" }

$Hostname       = "releem-agent-test-$OsVersion"
$ConfigPath     = "C:\ProgramData\ReleemAgent\releem.conf"
$ReleemUserPass = "ReleemNoPrompt!$(Get-Date -UFormat %s)"

function Get-ReleemConfigValue {
    param([string]$Path, [string]$Key)

    foreach ($line in Get-Content -Path $Path) {
        if ($line -match "^$([regex]::Escape($Key))=""?(.*?)""?$") {
            return $matches[1]
        }
    }

    throw "Key '$Key' not found in $Path"
}

Write-Info "=== Test 9: Reinstall rewrites config without prompt ==="
Write-Info "Hostname: $Hostname"

Remove-ReleemAgent
Remove-ReleemMySQLUser

Write-Info "Creating releem MySQL user for initial install..."
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

$env:RELEEM_HOSTNAME        = $Hostname
$env:RELEEM_MYSQL_LOGIN     = "releem"
$env:RELEEM_MYSQL_PASSWORD  = $ReleemUserPass
$env:RELEEM_MYSQL_HOST      = "127.0.0.1"
$env:RELEEM_MYSQL_PORT      = "3306"
$env:RELEEM_CRON_ENABLE     = "1"
$env:RELEEM_DB_MEMORY_LIMIT = "0"

Write-Info "Running initial install.ps1 with explicit credentials..."
& powershell.exe -ExecutionPolicy Bypass -File $InstallScript
$firstExit = $LASTEXITCODE

Assert-Zero "Initial install.ps1 exited successfully" $firstExit
Assert-FileExists "Initial install created releem.conf" $ConfigPath

Remove-Item Env:RELEEM_MYSQL_LOGIN -ErrorAction SilentlyContinue
Remove-Item Env:RELEEM_MYSQL_PASSWORD -ErrorAction SilentlyContinue

$secondRunCommand = "& { function global:Read-Host { param([string]`$Prompt) throw 'Read-Host should not be called during reinstall' }; & '$InstallScript' }"

Write-Info "Running install.ps1 a second time without login/password env..."
& powershell.exe -ExecutionPolicy Bypass -Command $secondRunCommand
$secondExit = $LASTEXITCODE

Assert-Zero "Second install.ps1 exited successfully without prompting" $secondExit
Assert-FileExists "Second install rewrote releem.conf" $ConfigPath
Assert-ServiceRunning "Second install left releem-agent service running" "releem-agent"

$currentMysqlUser = Get-ReleemConfigValue -Path $ConfigPath -Key 'mysql_user'
$currentMysqlPassword = Get-ReleemConfigValue -Path $ConfigPath -Key 'mysql_password'

Assert-MySQLCanConnect "Current releem.conf credentials can connect after reinstall" $currentMysqlUser $currentMysqlPassword

Show-Summary "Test 9: Reinstall rewrites config without prompt"
