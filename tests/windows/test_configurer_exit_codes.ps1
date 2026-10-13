# Offline checks: no database, service, administrator rights, or Pester required.
$ErrorActionPreference = 'Stop'
$scriptPath = Join-Path $PSScriptRoot '../../windows/releem.ps1'
$tokens = $null
$parseErrors = $null
$ast = [System.Management.Automation.Language.Parser]::ParseFile(
    (Resolve-Path $scriptPath).Path, [ref]$tokens, [ref]$parseErrors)
if ($parseErrors.Count) { throw ($parseErrors | Out-String) }
foreach ($name in @('Read-ReleemConfig', 'Set-ReleemConfigValue', 'Check-MySQLConfigDirectory', 'Check-MySQLConfigInclude', 'Invoke-ApplyConfig')) {
    $function = $ast.Find({ param($node)
        $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq $name
    }, $true)
    if (-not $function) { throw "Missing function: $name" }
    . ([scriptblock]::Create($function.Extent.Text))
}
function Write-Log { param($Message) }
function Find-MyIniPath { return $script:TestIniPath }
function Assert-Code { param($Expected, $Actual, $Label)
    if ($Expected -ne $Actual) { throw "${Label}: expected $Expected, got $Actual" }
}
$tempDir = Join-Path ([IO.Path]::GetTempPath()) ([guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $tempDir | Out-Null
try {
    $mysql_cnf_dir = ''
    Assert-Code 3 (Check-MySQLConfigDirectory) 'unset directory'
    $mysql_cnf_dir = Join-Path $tempDir 'absent'
    Assert-Code 3 (Check-MySQLConfigDirectory) 'missing directory'
    $mysql_cnf_dir = Join-Path $tempDir 'file.cnf'
    Set-Content $mysql_cnf_dir '[mysqld]'
    Assert-Code 3 (Check-MySQLConfigDirectory) 'file instead of directory'
    $mysql_cnf_dir = Join-Path $tempDir 'config with spaces'
    New-Item -ItemType Directory $mysql_cnf_dir | Out-Null
    Assert-Code 0 (Check-MySQLConfigDirectory) 'existing directory'
    $script:TestIniPath = Join-Path $tempDir 'my.ini'
    foreach ($content in @('[mysqld]', "# !includedir $mysql_cnf_dir", "!includedir ${mysql_cnf_dir}-other")) {
        Set-Content $script:TestIniPath $content
        Assert-Code 11 (Check-MySQLConfigInclude) "invalid include: $content"
    }
    foreach ($path in @($mysql_cnf_dir, ($mysql_cnf_dir -replace '\\', '/'), ($mysql_cnf_dir + '/'))) {
        Set-Content $script:TestIniPath "  !includedir `"$path`""
        Assert-Code 0 (Check-MySQLConfigInclude) "valid include: $path"
    }
    # Round trip the exact escaping used by install.ps1 and -Configure.
    $ConfigFilePath = Join-Path $tempDir 'releem.conf'
    $originalDirectory = $mysql_cnf_dir
    foreach ($directory in @('C:\ProgramData\Releem Agent\releem.conf.d', '\\server\share\releem.conf.d')) {
        foreach ($writer in @('installer', 'configure')) {
            if ($writer -eq 'installer') {
                Set-Content $ConfigFilePath ('mysql_cnf_dir="' + $directory.Replace('\', '\\') + '"')
            } else {
                Set-ReleemConfigValue -Key 'mysql_cnf_dir' -Value ($directory -replace '\\', '\\')
            }
            $mysql_cnf_dir = (Read-ReleemConfig)['mysql_cnf_dir']
            if ($mysql_cnf_dir -cne $directory) { throw "Path round trip failed: $writer / $directory" }
            foreach ($include in @($directory, $directory.Replace('\', '/'), ($directory.ToUpperInvariant() + '\'))) {
                Set-Content $script:TestIniPath "!includedir `"$include`""
                Assert-Code 0 (Check-MySQLConfigInclude) "persisted path: $writer / $include"
            }
            Set-Content $script:TestIniPath "!includedir ${directory}-other"
            Assert-Code 11 (Check-MySQLConfigInclude) 'persisted path rejects different directory'
        }
    }
    $mysql_cnf_dir = $originalDirectory
    $StagingCnfPath = Join-Path $tempDir 'recommendation.cnf'
    Set-Content $StagingCnfPath '[mysqld]'
    Set-Content $script:TestIniPath '[mysqld]'
    $script:ExitCode = 0
    Invoke-ApplyConfig -Interactive $false -RestartService $false -ConfigurationApplyMode dynamic
    Assert-Code 11 $script:ExitCode 'apply rejects missing include before modification'
    $mysql_cnf_dir = Join-Path $tempDir 'absent'
    $script:ExitCode = 0
    Invoke-ApplyConfig -Interactive $false -RestartService $false -ConfigurationApplyMode dynamic
    Assert-Code 3 $script:ExitCode 'apply rejects missing directory'
    # Execute the actual rollback branch without entering the Windows bootstrap.
    $rollbackBranch = $ast.Find({ param($node)
        $node -is [System.Management.Automation.Language.IfStatementAst] -and
        $node.Clauses[0].Item1.Extent.Text -eq '$Rollback'
    }, $true)
    if (-not $rollbackBranch) { throw 'Missing rollback branch' }
    $script:ExitCode = 0
    & ([scriptblock]::Create($rollbackBranch.Clauses[0].Item2.Extent.Text.Trim().TrimStart('{').TrimEnd('}')))
    Assert-Code 3 $script:ExitCode 'rollback rejects missing directory'
    Remove-Item $StagingCnfPath
    $script:ExitCode = 0
    Invoke-ApplyConfig -Interactive $false -RestartService $false -ConfigurationApplyMode dynamic
    Assert-Code 1 $script:ExitCode 'missing recommendation retains code 1'
    $script:TestIniPath = $null
    Assert-Code 0 (Check-MySQLConfigInclude) 'undiscovered my.ini follows Linux behavior'
    Write-Host 'PASS: Windows configurer directory and include exit codes'
} finally {
    Remove-Item $tempDir -Recurse -Force
}
