# Bootstrap script for GCP Windows test VMs.
# Installs MySQL/MariaDB, loads world DB, unpacks test payload and runs tests without SSH.

$ErrorActionPreference = 'Stop'
# On PowerShell 7, prevent native stderr output from becoming terminating errors.
if (Get-Variable -Name PSNativeCommandUseErrorActionPreference -ErrorAction SilentlyContinue) {
    $PSNativeCommandUseErrorActionPreference = $false
}

$Hostname       = '${hostname}'
$OsVersion      = '${os_version}'
$DbVersion      = '${db_version}'
$DbRootPass     = '${db_root_password}'
$ReleemApiKey   = '${releem_api_key}'
$TestSelection  = '${test_selection}'
$TestPayloadB64 = '${test_payload_b64}'

$LogFile = 'C:/bootstrap.log'
function Write-Log {
    param([string]$msg)
    $line = '[' + (Get-Date -Format 'yyyy-MM-dd HH:mm:ss') + '] ' + $msg
    Add-Content -Path $LogFile -Value $line
    Write-Host $line
}

function Download-File {
    param(
        [string]$Url,
        [string]$OutFile
    )
    # curl.exe behaves more reliably than Invoke-WebRequest for large archives on Windows startup.
    & curl.exe -fsSL --retry 5 --retry-delay 5 --connect-timeout 20 --max-time 1800 -o $OutFile $Url
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path $OutFile)) {
        throw ('Download failed ({0}) -> {1}' -f $Url, $OutFile)
    }
}

trap {
    Write-Log ('FATAL: {0}' -f $_)
    Write-Log 'RELEEM_TEST_RESULT:FAIL'
    exit 1
}

Write-Log ('Bootstrap started: DB={0}, Hostname={1}' -f $DbVersion, $Hostname)
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$mysqlDir = $null
$DbServiceName = $null

if ($DbVersion -like 'mysql*') {
    if ($DbVersion -eq 'mysql-8.0') {
        $zipUrl = 'https://cdn.mysql.com/archives/mysql-8.0/mysql-8.0.39-winx64.zip'
    } else {
        $zipUrl = 'https://cdn.mysql.com/archives/mysql-8.4/mysql-8.4.0-winx64.zip'
    }

    $mysqlZip = 'C:/mysql-server.zip'
    Write-Log ('Downloading MySQL from {0}' -f $zipUrl)
    Download-File -Url $zipUrl -OutFile $mysqlZip
    Expand-Archive -Path $mysqlZip -DestinationPath 'C:/MySQL' -Force
    $mysqlDir = (Get-ChildItem 'C:/MySQL' -Directory | Select-Object -First 1).FullName
    if (-not $mysqlDir) {
        throw 'MySQL extraction failed'
    }

    # MySQL ZIP binaries require VC++ runtime on clean Windows images.
    $vcRedistExe = 'C:/vc_redist.x64.exe'
    Write-Log 'Installing Microsoft VC++ runtime...'
    Download-File -Url 'https://aka.ms/vs/17/release/vc_redist.x64.exe' -OutFile $vcRedistExe
    $vcProc = Start-Process -FilePath $vcRedistExe -ArgumentList '/install /quiet /norestart' -Wait -PassThru
    if ($vcProc.ExitCode -ne 0 -and $vcProc.ExitCode -ne 3010) {
        throw ('VC++ runtime installation failed: {0}' -f $vcProc.ExitCode)
    }

    $basedirFwd = $mysqlDir -replace '\\', '/'
    $myIni = Join-Path $mysqlDir 'my.ini'
    Set-Content $myIni -Value '[mysqld]' -Encoding ASCII
    Add-Content $myIni -Value ('basedir={0}' -f $basedirFwd) -Encoding ASCII
    Add-Content $myIni -Value 'datadir=C:/ProgramData/MySQL/Data' -Encoding ASCII
    Add-Content $myIni -Value 'port=3306' -Encoding ASCII

    New-Item -ItemType Directory -Path 'C:/ProgramData/MySQL/Data' -Force | Out-Null
    $mysqldExe = Join-Path (Join-Path $mysqlDir 'bin') 'mysqld.exe'
    & $mysqldExe --initialize-insecure --datadir='C:/ProgramData/MySQL/Data' --basedir=$mysqlDir 2>&1 | ForEach-Object { Write-Log $_ }
    if ($LASTEXITCODE -ne 0) {
        throw ('mysqld --initialize-insecure failed: {0}' -f $LASTEXITCODE)
    }
    & $mysqldExe --install MySQL --defaults-file=$myIni 2>&1 | ForEach-Object { Write-Log $_ }
    if ($LASTEXITCODE -ne 0 -and -not (Get-Service -Name 'MySQL' -ErrorAction SilentlyContinue)) {
        throw ('mysqld --install failed: {0}' -f $LASTEXITCODE)
    }
} elseif ($DbVersion -like 'mariadb*') {
    $url = 'https://downloads.mariadb.org/rest-api/mariadb/10.11/mariadb-10.11.7-winx64.msi'
    $msi = 'C:/mariadb-installer.msi'
    Write-Log ('Downloading MariaDB installer from {0}' -f $url)
    Download-File -Url $url -OutFile $msi
    $msiArgs = '/i "{0}" /qn SERVICENAME=MySQL PORT=3306 PASSWORD="{1}"' -f $msi, $DbRootPass
    $msiProc = Start-Process msiexec.exe -ArgumentList $msiArgs -Wait -NoNewWindow -PassThru
    if ($msiProc.ExitCode -ne 0 -and $msiProc.ExitCode -ne 3010) {
        throw ('MariaDB MSI install failed: {0}' -f $msiProc.ExitCode)
    }
    $mysqlDir = (Get-ChildItem 'C:/Program Files' -Directory | Where-Object { $_.Name -like 'MariaDB*' } | Select-Object -First 1).FullName
    if (-not $mysqlDir) {
        throw 'MariaDB installation failed: install directory not found'
    }
}

# Pick whatever DB service name is actually present on the host.
foreach ($candidate in @('MySQL', 'MySQL80', 'MySQL84', 'MariaDB', 'mariadb', 'mysqld')) {
    if (Get-Service -Name $candidate -ErrorAction SilentlyContinue) {
        $DbServiceName = $candidate
        break
    }
}
if (-not $DbServiceName) {
    throw 'No DB service found after installation'
}

Write-Log ('Starting DB service: {0}' -f $DbServiceName)
Start-Service -Name $DbServiceName
Start-Sleep -Seconds 15
$svc = Get-Service -Name $DbServiceName -ErrorAction SilentlyContinue
if (-not $svc -or $svc.Status -ne 'Running') {
    throw ('DB service failed to start: {0}' -f $DbServiceName)
}

if ($mysqlDir) {
    $binDir = Join-Path $mysqlDir 'bin'
    $env:Path = $binDir + ';' + $env:Path
    $machinePath = [Environment]::GetEnvironmentVariable('Path', 'Machine')
    if ($machinePath -notlike "*$binDir*") {
        [Environment]::SetEnvironmentVariable('Path', ($machinePath + ';' + $binDir), 'Machine')
    }
    Write-Log ('MySQL added to PATH: {0}' -f $binDir)
}

if ($DbVersion -like 'mysql*' -and $mysqlDir) {
    $mysqlExe = Join-Path (Join-Path $mysqlDir 'bin') 'mysql.exe'
    & $mysqlExe --protocol=tcp -h 127.0.0.1 -P 3306 -u root -e ("ALTER USER 'root'@'localhost' IDENTIFIED BY '{0}'; FLUSH PRIVILEGES;" -f $DbRootPass) 2>&1 | ForEach-Object { Write-Log $_ }
    if ($LASTEXITCODE -ne 0) {
        throw ('Failed to set MySQL root password: {0}' -f $LASTEXITCODE)
    }
    $env:MYSQL_PWD = $DbRootPass
    & $mysqlExe --protocol=tcp -h 127.0.0.1 -P 3306 -u root -e "SELECT 1;" 2>&1 | ForEach-Object { Write-Log $_ }
    Remove-Item Env:MYSQL_PWD -ErrorAction SilentlyContinue
    if ($LASTEXITCODE -ne 0) {
        throw ('MySQL root password verification failed: {0}' -f $LASTEXITCODE)
    }
}

Write-Log 'Loading world sample database (best effort)...'
try {
    $worldUrl = 'https://downloads.mysql.com/docs/world-db.zip'
    $worldZip = 'C:/world-db.zip'
    Download-File -Url $worldUrl -OutFile $worldZip
    Expand-Archive -Path $worldZip -DestinationPath 'C:/world-db' -Force
    $worldSql = 'C:/world-db/world-db/world.sql'
    if ($mysqlDir -and (Test-Path $worldSql)) {
        $mysqlExe = Join-Path (Join-Path $mysqlDir 'bin') 'mysql.exe'
        Get-Content -Path $worldSql -Raw | & $mysqlExe -u root -p"$DbRootPass" 2>&1 | ForEach-Object { Write-Log $_ }
        Write-Log 'World database loaded'
    } else {
        Write-Log 'World database skipped: mysql or world.sql missing'
    }
} catch {
    Write-Log ('World database load skipped due to error: {0}' -f $_)
}

Write-Log 'Writing Windows test payload...'
if ([string]::IsNullOrWhiteSpace($TestPayloadB64)) {
    throw 'Test payload is empty'
}
$payloadZip = 'C:/releem-tests-payload.zip'
[IO.File]::WriteAllBytes($payloadZip, [Convert]::FromBase64String($TestPayloadB64))
$testDir = 'C:/releem_tests'
New-Item -ItemType Directory -Path $testDir -Force | Out-Null
Expand-Archive -Path $payloadZip -DestinationPath $testDir -Force

$env:RELEEM_API_KEY = $ReleemApiKey
$env:MYSQL_ROOT_PASSWORD = $DbRootPass
$env:OS_VERSION = $OsVersion
$env:INSTALL_SCRIPT = 'C:\releem_tests\install.ps1'
$env:CONFIGURER_SCRIPT = 'C:\releem_tests\mysqlconfigurer.ps1'

Write-Log ('Running Windows test suite: {0}' -f $TestSelection)
& powershell.exe -ExecutionPolicy Bypass -NonInteractive -File 'C:\releem_tests\run_all.ps1' -Test $TestSelection
$testExit = $LASTEXITCODE
if ($testExit -eq 0) {
    Write-Log 'Windows tests passed'
    Write-Log 'RELEEM_TEST_RESULT:PASS'
} else {
    Write-Log ('Windows tests failed: {0}' -f $testExit)
    Write-Log 'RELEEM_TEST_RESULT:FAIL'
    exit $testExit
}

if ($env:COMPUTERNAME -ne $Hostname) {
    Rename-Computer -NewName $Hostname -Force -ErrorAction SilentlyContinue
}
Set-Content -Path 'C:/bootstrap_complete.txt' -Value ((Get-Date).ToString('yyyy-MM-dd HH:mm:ss'))
Write-Log 'Bootstrap complete'
Write-Log 'RELEEM_BOOTSTRAP_COMPLETE'
