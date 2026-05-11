# Test 7: mysqlconfigurer.ps1 -Apply -NonInteractive -NoRestart copies config without restarting MySQL.

param()

. "$PSScriptRoot\helpers.ps1"

$env:MYSQL_ROOT_PASSWORD = if ($env:MYSQL_ROOT_PASSWORD) { $env:MYSQL_ROOT_PASSWORD } else { throw "MYSQL_ROOT_PASSWORD must be set" }

$DbService      = Get-MySQLServiceName
$StagingDir     = "C:\ProgramData\ReleemAgent\conf.d"
$StagingConfig  = "$StagingDir\z_aiops_mysql.cnf"
$BackupConfig   = "$StagingDir\z_aiops_mysql.cnf.bkp"
$ReleemConfFile = "C:\ProgramData\ReleemAgent\releem.conf"

function Get-ServiceSnapshot {
    param([string]$ServiceName)

    $service = Get-CimInstance Win32_Service -Filter "Name='$ServiceName'"
    if (-not $service) {
        throw "Service '$ServiceName' was not found"
    }

    $process = Get-Process -Id $service.ProcessId
    return @{
        ProcessId = [string]$service.ProcessId
        StartTime = $process.StartTime.ToString('o')
    }
}

function Get-ReleemConfigValue {
    param([string]$Path, [string]$Key)

    foreach ($line in Get-Content -Path $Path) {
        if ($line -match "^$([regex]::Escape($Key))=""?(.*?)""?$") {
            return $matches[1]
        }
    }

    throw "Key '$Key' not found in $Path"
}

Write-Info "=== Test 7: Apply configuration without restart ==="

Assert-ServiceRunning "Pre: DB service running" $DbService
Assert-ServiceRunning "Pre: releem-agent running" "releem-agent"
Assert-FileExists "Pre: releem.conf present" $ReleemConfFile

if (Test-Path $ReleemConfFile) {
    $releemConf = Get-Content $ReleemConfFile -Raw
    if ($releemConf -notmatch '(?m)^mysql_cnf_dir=') {
        Add-Content -Path $ReleemConfFile -Value "mysql_cnf_dir=`"$StagingDir`""
    }
    if ($releemConf -notmatch '(?m)^mysql_restart_service=') {
        Add-Content -Path $ReleemConfFile -Value "mysql_restart_service=`"net stop $DbService && net start $DbService`""
    }
}

if (-not (Test-Path $StagingConfig)) {
    Write-Info "Creating local recommended config stub for no-restart test..."
    Set-Content -Path $StagingConfig -Value '[mysqld]' -Encoding ASCII
    Add-Content -Path $StagingConfig -Value 'max_connections=222' -Encoding ASCII
}

Write-Info "Running mysqlconfigurer.ps1 -Configure..."
& powershell.exe -ExecutionPolicy Bypass -File $ConfigurerScript -Configure
$configureExit = $LASTEXITCODE
Assert-Zero "mysqlconfigurer.ps1 -Configure exited successfully" $configureExit

$liveConfigDir  = Get-ReleemConfigValue -Path $ReleemConfFile -Key 'mysql_cnf_dir'
$liveConfigPath = Join-Path $liveConfigDir 'z_aiops_mysql.cnf'

Remove-Item -Path $BackupConfig -Force -ErrorAction SilentlyContinue
$beforeSnapshot = Get-ServiceSnapshot -ServiceName $DbService

Write-Info "Running mysqlconfigurer.ps1 -Apply -NonInteractive -NoRestart..."
& powershell.exe -ExecutionPolicy Bypass -File $ConfigurerScript -Apply -NonInteractive -NoRestart
$applyExit = $LASTEXITCODE

$afterSnapshot = Get-ServiceSnapshot -ServiceName $DbService

Assert-Zero "mysqlconfigurer.ps1 -Apply -NonInteractive -NoRestart exited successfully" $applyExit
Assert-FileExists "Staging config file exists" $StagingConfig
Assert-FileExists "Live config file copied" $liveConfigPath
Assert-FileExists "Backup file preserved when restart is skipped" $BackupConfig
Assert-ServiceRunning "DB service still running" $DbService
Assert-ServiceRunning "releem-agent still running" "releem-agent"
Assert-Equal "MySQL process id unchanged without restart" $beforeSnapshot.ProcessId $afterSnapshot.ProcessId
Assert-Equal "MySQL process start time unchanged without restart" $beforeSnapshot.StartTime $afterSnapshot.StartTime

Show-Summary "Test 7: Apply configuration without restart"
