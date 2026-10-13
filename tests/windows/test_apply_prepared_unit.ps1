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
function Find-MyIniPath { return $null }
function Write-Log { param([string]$Message) }
function Check-DbVersion { return 0 }
function Read-Host { throw 'Unexpected interactive prompt' }
function Restart-MySqlService { $script:restartCalls++; return $script:restartResult }
function Send-TestEvent {
    if ($args -contains '-c') { throw 'Apply must not download a different configuration' }
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
try {
    $script:ExitCode = 0
    Invoke-ApplyConfig -Interactive $false -RestartService $true
    Assert-Equal $script:ExitCode 1
    Assert-Equal $script:restartCalls 0
    Assert-Equal (Test-Path $BackupCnfPath) $false

    [IO.File]::WriteAllText($StagingCnfPath, "[mysqld]`nmax_connections=200`n")
    [IO.File]::WriteAllText($livePath, "[mysqld]`nmax_connections=100`n")
    [IO.File]::WriteAllText($BackupCnfPath, 'original backup')
    $script:ExitCode = 0
    Invoke-ApplyConfig -Interactive $false -RestartService $false
    Assert-Equal $script:ExitCode 0
    Assert-Equal $script:restartCalls 0
    Assert-Equal ([IO.File]::ReadAllText($BackupCnfPath)) 'original backup'
    Assert-Equal ([IO.File]::ReadAllText($livePath)) ([IO.File]::ReadAllText($StagingCnfPath))

    Invoke-ApplyConfig -Interactive $true -RestartService $true
    Assert-Equal $script:restartCalls 0
    Assert-Equal ([IO.File]::ReadAllText($BackupCnfPath)) 'original backup'

    $script:restartResult = 7
    Invoke-ApplyConfig -Interactive $false -RestartService $true
    Assert-Equal $script:ExitCode 7
    Assert-Equal ([IO.File]::ReadAllText($BackupCnfPath)) 'original backup'

    $script:restartResult = 0
    $script:ExitCode = 0
    Invoke-ApplyConfig -Interactive $false -RestartService $true
    Assert-Equal $script:ExitCode 0
    Assert-Equal (Test-Path $BackupCnfPath) $false

    $mysql_cnf_dir = Join-Path $root 'missing'
    Invoke-ApplyConfig -Interactive $false -RestartService $true
    Assert-Equal $script:ExitCode 3
    Write-Host 'Prepared configuration apply tests passed.'
} finally {
    Remove-Item -LiteralPath $root -Recurse -Force
}
