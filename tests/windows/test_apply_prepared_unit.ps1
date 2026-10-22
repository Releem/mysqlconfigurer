# Isolated apply tests: no Administrator, MySQL service, Agent or network required.
$ErrorActionPreference = 'Stop'
$scriptPath = Join-Path $PSScriptRoot '../../windows/releem.ps1'
$tokens = $null
$parseErrors = $null
$ast = [System.Management.Automation.Language.Parser]::ParseFile((Resolve-Path $scriptPath), [ref]$tokens, [ref]$parseErrors)
if ($parseErrors.Count -gt 0) { throw ($parseErrors | Out-String) }
foreach ($name in @('Check-MySQLConfigDirectory', 'Check-MySQLConfigInclude', 'Invoke-ApplyConfig')) {
    $function = $ast.Find({ param($node) $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq $name }, $true)
    if (-not $function) { throw "Missing function: $name" }
    Invoke-Expression $function.Extent.Text
}
$rollbackBranch = $ast.Find({ param($node)
    $node -is [Management.Automation.Language.IfStatementAst] -and
    $node.Clauses[0].Item1.Extent.Text -eq '$Rollback'
}, $true)
if (-not $rollbackBranch) { throw 'Missing rollback branch' }
$rollbackText = $rollbackBranch.Clauses[0].Item2.Extent.Text
$rollbackBody = [scriptblock]::Create($rollbackText.Substring(1, $rollbackText.Length - 2))
function Find-MyIniPath { return $null }
function Write-Log { param([string]$Message) if ($env:RELEEM_TEST_VERBOSE) { Write-Host $Message } }
function Check-DbVersion { return 0 }
function Read-Host { throw 'Unexpected interactive prompt' }
function Restart-MySqlService { $script:restartCalls++; return $script:restartResult }
# Windows Server 2012 ships PowerShell 3.0 without Get-FileHash.
function Get-FileHash { throw 'Get-FileHash is unavailable on PowerShell 3.0' }
function Send-TestEvent {
    if ($args -contains '-c') { throw 'Apply must never download configuration' }
    $global:releemPreparedTestEvents += ($args -join ' ')
    $global:LASTEXITCODE = 0
}
function Assert-Equal($actual, $expected) { if ($actual -cne $expected) { throw "Expected [$expected], got [$actual]" } }
$root = Join-Path ([IO.Path]::GetTempPath()) ([guid]::NewGuid().ToString())
New-Item -Path $root -ItemType Directory | Out-Null
$mysql_cnf_dir = Join-Path $root 'live'
New-Item -Path $mysql_cnf_dir -ItemType Directory | Out-Null
$DbConfigFileName = 'z_aiops_mysql.cnf'
$StagingCnfPath = Join-Path $root $DbConfigFileName
$BackupCnfPath = "$StagingCnfPath.bkp"
$livePath = Join-Path $mysql_cnf_dir $DbConfigFileName
$AgentBinaryPath = Join-Path $root 'agent-stub.ps1'
[IO.File]::WriteAllText($AgentBinaryPath, 'Send-TestEvent @args')
$script:restartCalls = 0
$script:restartResult = 0
$global:releemPreparedTestEvents = @()
$ApplyMode = 'full'
$previousForce = $env:RELEEM_FORCE_RESTART
$previousRollbackConfirm = $env:RELEEM_ROLLBACK_CONFIRM
Remove-Item Env:RELEEM_FORCE_RESTART -ErrorAction SilentlyContinue
try {
    $script:ExitCode = 0
    Invoke-ApplyConfig -Interactive $false -RestartService $true
    Assert-Equal $script:ExitCode 1
    Assert-Equal $script:restartCalls 0
    Assert-Equal (Test-Path $BackupCnfPath) $false

    [IO.File]::WriteAllText($StagingCnfPath, "[mysqld]`nmax_connections=200`n")
    [IO.File]::WriteAllText($livePath, "[mysqld]`nmax_connections=100`n")
    $script:ExitCode = 0
    Invoke-ApplyConfig -Interactive $false -RestartService $false
    Assert-Equal $script:ExitCode 0
    Assert-Equal $script:restartCalls 0
    Assert-Equal (Test-Path $BackupCnfPath) $false
    Assert-Equal ([IO.File]::ReadAllText($livePath)) ([IO.File]::ReadAllText($StagingCnfPath))

    # Completed identical files skip restart and do not create a backup.
    Invoke-ApplyConfig -Interactive $true -RestartService $true
    Invoke-ApplyConfig -Interactive $false -RestartService $true
    Assert-Equal $script:restartCalls 0
    Assert-Equal (Test-Path $BackupCnfPath) $false
    Assert-Equal $global:releemPreparedTestEvents.Count 1

    foreach ($restartFailure in @(6, 7, 19)) {
        [IO.File]::WriteAllText($livePath, "[mysqld]`nmax_connections=100`n")
        [IO.File]::WriteAllText($BackupCnfPath, 'older backup')
        [IO.File]::WriteAllText($StagingCnfPath, "[mysqld]`nmax_connections=200`n")
        $script:restartResult = $restartFailure
        $script:ExitCode = 0
        Invoke-ApplyConfig -Interactive $false -RestartService $true
        Assert-Equal $script:ExitCode $restartFailure
        Assert-Equal ([IO.File]::ReadAllText($BackupCnfPath)) "[mysqld]`nmax_connections=100`n"

        # Even an identical retry restarts; each attempt backs up its current file.
        $restartCallsBefore = $script:restartCalls
        $script:ExitCode = 0
        Invoke-ApplyConfig -Interactive $false -RestartService $true
        Assert-Equal $script:ExitCode $restartFailure
        Assert-Equal $script:restartCalls ($restartCallsBefore + 1)
        Assert-Equal ([IO.File]::ReadAllText($BackupCnfPath)) "[mysqld]`nmax_connections=200`n"

        [IO.File]::WriteAllText($StagingCnfPath, "[mysqld]`nmax_connections=300`n")
        $script:ExitCode = 0
        Invoke-ApplyConfig -Interactive $false -RestartService $true -ForceRestart $true
        Assert-Equal $script:ExitCode $restartFailure
        Assert-Equal ([IO.File]::ReadAllText($BackupCnfPath)) "[mysqld]`nmax_connections=200`n"

        $env:RELEEM_ROLLBACK_CONFIRM = '1'
        $script:ExitCode = 0
        & $rollbackBody
        Assert-Equal $script:ExitCode $restartFailure
        Assert-Equal ([IO.File]::ReadAllText($livePath)) "[mysqld]`nmax_connections=200`n"
        Assert-Equal ([IO.File]::ReadAllText($BackupCnfPath)) "[mysqld]`nmax_connections=200`n"

        $script:restartResult = 0
        $script:ExitCode = 0
        & $rollbackBody
        Assert-Equal $script:ExitCode 0
        Assert-Equal (Test-Path $BackupCnfPath) $false
    }

    # A successful retry removes its backup and notifies the platform.
    [IO.File]::WriteAllText($livePath, "[mysqld]`nmax_connections=300`n")
    [IO.File]::WriteAllText($StagingCnfPath, "[mysqld]`nmax_connections=300`n")
    [IO.File]::WriteAllText($BackupCnfPath, "[mysqld]`nmax_connections=200`n")
    $script:ExitCode = 0
    $restartCallsBefore = $script:restartCalls
    $eventsBefore = $global:releemPreparedTestEvents.Count
    Invoke-ApplyConfig -Interactive $false -RestartService $true
    Assert-Equal $script:ExitCode 0
    Assert-Equal $script:restartCalls ($restartCallsBefore + 1)
    Assert-Equal $global:releemPreparedTestEvents.Count ($eventsBefore + 1)
    Assert-Equal (Test-Path $BackupCnfPath) $false

    # Full apply and managed rollback restart even for identical files.
    $restartCallsBefore = $script:restartCalls
    Invoke-ApplyConfig -Interactive $false -RestartService $true -ForceRestart $true
    Assert-Equal $script:restartCalls ($restartCallsBefore + 1)
    Assert-Equal (Test-Path $BackupCnfPath) $false

    foreach ($mode in @('full', 'dynamic')) {
        $ApplyMode = $mode
        [IO.File]::WriteAllText($BackupCnfPath, 'older backup')
        [IO.File]::WriteAllText($StagingCnfPath, "[mysqld]`n# downloaded by Go: $mode`n")
        $script:ExitCode = 0
        Invoke-ApplyConfig -Interactive $false -RestartService $false
        Assert-Equal $script:ExitCode 0
        Assert-Equal ([IO.File]::ReadAllText($livePath)) ([IO.File]::ReadAllText($StagingCnfPath))
        Assert-Equal (Test-Path $BackupCnfPath) $false
        Assert-Equal $global:releemPreparedTestEvents[-1] '--event=config_applied'
    }

    $env:RELEEM_FORCE_RESTART = '1'
    $restartCallsBefore = $script:restartCalls
    Invoke-ApplyConfig -Interactive $false -RestartService $true
    Assert-Equal $script:restartCalls ($restartCallsBefore + 1)
    Assert-Equal (Test-Path $BackupCnfPath) $false
    Remove-Item Env:RELEEM_FORCE_RESTART

    # Missing staging and a directory in its place preserve live config and backup.
    [IO.File]::WriteAllText($BackupCnfPath, 'original baseline')
    $liveBefore = [Convert]::ToBase64String([IO.File]::ReadAllBytes($livePath))
    $backupBefore = [Convert]::ToBase64String([IO.File]::ReadAllBytes($BackupCnfPath))
    $eventsBefore = $global:releemPreparedTestEvents.Count
    $restartCallsBefore = $script:restartCalls
    Remove-Item $StagingCnfPath
    foreach ($failure in @('missing', 'directory')) {
        if ($failure -eq 'directory') { New-Item $StagingCnfPath -ItemType Directory | Out-Null }
        $script:ExitCode = 0
        Invoke-ApplyConfig -Interactive $false -RestartService $true
        Assert-Equal $script:ExitCode 1
        Assert-Equal ([Convert]::ToBase64String([IO.File]::ReadAllBytes($livePath))) $liveBefore
        Assert-Equal ([Convert]::ToBase64String([IO.File]::ReadAllBytes($BackupCnfPath))) $backupBefore
        Assert-Equal $script:restartCalls $restartCallsBefore
        Assert-Equal $global:releemPreparedTestEvents.Count $eventsBefore
    }
    Remove-Item $StagingCnfPath

    # Baseline and BOM semantics stay canonical when Go supplies a BOM file.
    # A failed restart keeps the baseline backup so it can be inspected.
    Remove-Item $livePath, $BackupCnfPath
    $bomUtf8 = New-Object Text.UTF8Encoding $true
    [IO.File]::WriteAllText($StagingCnfPath, "[mysqld]`r`nmax_connections=888`r`n", $bomUtf8)
    $script:restartResult = 7
    $script:ExitCode = 0
    Invoke-ApplyConfig -Interactive $false -RestartService $true
    Assert-Equal $script:ExitCode 7
    $script:restartResult = 0
    Assert-Equal ([IO.File]::ReadAllText($BackupCnfPath)) "[mysqld]`r`n"
    Assert-Equal ([IO.File]::ReadAllBytes($BackupCnfPath)[0]) 91
    Assert-Equal ([IO.File]::ReadAllBytes($livePath)[0]) 91
    Assert-Equal ([IO.File]::ReadAllText($livePath)) ([IO.File]::ReadAllText($StagingCnfPath))

    $mysql_cnf_dir = Join-Path $root 'missing'
    Invoke-ApplyConfig -Interactive $false -RestartService $true
    Assert-Equal $script:ExitCode 3
    Write-Host 'Canonical configuration apply tests passed.'
} finally {
    $env:RELEEM_FORCE_RESTART = $previousForce
    $env:RELEEM_ROLLBACK_CONFIRM = $previousRollbackConfirm
    Remove-Variable -Name releemPreparedTestEvents -Scope Global -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $root -Recurse -Force
}
