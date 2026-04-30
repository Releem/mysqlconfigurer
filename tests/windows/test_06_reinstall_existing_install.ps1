# Test 6: Re-running install.ps1 on an existing installation should succeed.

param()

. "$PSScriptRoot\helpers.ps1"

$env:RELEEM_API_KEY       = if ($env:RELEEM_API_KEY) { $env:RELEEM_API_KEY } else { throw "RELEEM_API_KEY must be set" }
$env:MYSQL_ROOT_PASSWORD  = if ($env:MYSQL_ROOT_PASSWORD) { $env:MYSQL_ROOT_PASSWORD } else { throw "MYSQL_ROOT_PASSWORD must be set" }
$OsVersion                = if ($env:OS_VERSION) { $env:OS_VERSION } else { throw "OS_VERSION must be set" }

$Hostname = "releem-agent-test-$OsVersion"
$ReleemUserPass = "ReleemReinstall!$(Get-Date -UFormat %s)"
$configPath = "C:\ProgramData\ReleemAgent\releem.conf"

Write-Info "=== Test 6: Re-running install.ps1 on an existing installation ==="
Write-Info "Hostname: $Hostname"

Remove-ReleemAgent
Remove-ReleemMySQLUser

Write-Info "Creating releem MySQL user for repeatable reinstall test..."
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

$env:RELEEM_HOSTNAME            = $Hostname
$env:RELEEM_MYSQL_LOGIN         = "releem"
$env:RELEEM_MYSQL_PASSWORD      = $ReleemUserPass
$env:RELEEM_MYSQL_HOST          = "127.0.0.1"
$env:RELEEM_MYSQL_PORT          = "3306"
$env:RELEEM_CRON_ENABLE         = "1"
$env:RELEEM_DB_MEMORY_LIMIT     = "0"
$env:RELEEM_QUERY_OPTIMIZATION  = "true"

Write-Info "Running initial install.ps1..."
& powershell.exe -ExecutionPolicy Bypass -File $InstallScript
$firstExit = $LASTEXITCODE

Assert-Zero "Initial install.ps1 exited successfully" $firstExit
Assert-FileExists "Initial install created releem.conf" $configPath
Assert-ServiceRunning "Initial install started releem-agent service" "releem-agent"

Write-Info "Running install.ps1 a second time against existing installation..."
& powershell.exe -ExecutionPolicy Bypass -File $InstallScript
$secondExit = $LASTEXITCODE

Assert-Zero "Second install.ps1 exited successfully" $secondExit
Assert-FileExists "Second install kept releem-agent.exe present" "C:\Program Files\ReleemAgent\releem-agent.exe"
Assert-ServiceRunning "Second install left releem-agent service running" "releem-agent"

Show-Summary "Test 6: Re-running install.ps1 on existing installation"
