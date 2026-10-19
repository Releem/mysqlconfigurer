# Offline Windows configurer checks. No database, service changes, administrator
# privileges, or Pester required. Test 5 invokes this suite on Windows Server.
param(
    [ValidateSet('All', 'ExitCodes', 'Legacy', 'Update', 'InstallServices')]
    [string]$Check = 'All',
    [string]$WindowsScriptDir = (Join-Path $PSScriptRoot '../../windows')
)

$ErrorActionPreference = 'Stop'
$root = $WindowsScriptDir
$engine = (Get-Process -Id $PID).Path
$asts = @{}

function Get-ConfigurerAst {
    param([string]$Name)
    if (-not $asts.ContainsKey($Name)) {
        $tokens = $null; $parseErrors = $null
        $path = (Resolve-Path (Join-Path $root $Name)).Path
        $asts[$Name] = [Management.Automation.Language.Parser]::ParseFile($path, [ref]$tokens, [ref]$parseErrors)
        if ($parseErrors.Count) { throw ($parseErrors | Out-String) }
    }
    return $asts[$Name]
}

function ConvertTo-TestScriptBlock {
    param($Node, [switch]$Body)
    $text = $Node.Extent.Text
    if ($Body) { $text = $text.Substring(1, $text.Length - 2) }
    return [scriptblock]::Create($text)
}

function Assert-Code {
    param($Expected, $Actual, $Label)
    if ($Expected -ne $Actual) { throw "${Label}: expected $Expected, got $Actual" }
}

function Invoke-TestScript {
    param([string]$Path, [string[]]$Arguments = @(), [int]$ExpectedExit = 0, [string]$Pattern)
    # Windows PowerShell 5.1 converts native stderr to error records. Capture
    # expected failures without letting the caller's Stop preference abort us.
    $previousPreference = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        $output = & $engine -NoProfile -File $Path @Arguments 2>&1 | Out-String
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousPreference
    }
    Assert-Code $ExpectedExit $exitCode $Path
    if ($Pattern -and $output -notmatch $Pattern) {
        throw "${Path}: expected output matching '$Pattern', got: $output"
    }
}

function Test-ConfigurerExitCodes {
    param($temp)
    $ast = Get-ConfigurerAst releem.ps1
    foreach ($name in @('Read-ReleemConfig', 'Set-ReleemConfigValue', 'Check-MySQLConfigDirectory', 'Check-MySQLConfigInclude', 'Invoke-ApplyConfig')) {
        $function = $ast.Find({ param($node)
            $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq $name
        }, $true)
        if (-not $function) { throw "Missing function: $name" }
        . (ConvertTo-TestScriptBlock $function)
    }
    function Write-Log { param($Message) }
    function Find-MyIniPath { return $script:TestIniPath }
    $mysql_cnf_dir = ''
    Assert-Code 3 (Check-MySQLConfigDirectory) 'unset directory'
    $mysql_cnf_dir = Join-Path $temp 'absent'
    Assert-Code 3 (Check-MySQLConfigDirectory) 'missing directory'
    $mysql_cnf_dir = Join-Path $temp 'file.cnf'
    Set-Content $mysql_cnf_dir '[mysqld]'
    Assert-Code 3 (Check-MySQLConfigDirectory) 'file instead of directory'
    $mysql_cnf_dir = Join-Path $temp 'config with spaces'
    New-Item -ItemType Directory $mysql_cnf_dir | Out-Null
    Assert-Code 0 (Check-MySQLConfigDirectory) 'existing directory'
    $script:TestIniPath = Join-Path $temp 'my.ini'
    foreach ($content in @('[mysqld]', "# !includedir $mysql_cnf_dir", "!includedir ${mysql_cnf_dir}-other")) {
        Set-Content $script:TestIniPath $content
        Assert-Code 11 (Check-MySQLConfigInclude) "invalid include: $content"
    }
    foreach ($path in @($mysql_cnf_dir, ($mysql_cnf_dir -replace '\\', '/'), ($mysql_cnf_dir + '/'))) {
        Set-Content $script:TestIniPath "  !includedir `"$path`""
        Assert-Code 0 (Check-MySQLConfigInclude) "valid include: $path"
    }
    # Round trip the exact escaping used by install.ps1 and -Configure.
    $ConfigFilePath = Join-Path $temp 'releem.conf'
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
    $StagingCnfPath = Join-Path $temp 'recommendation.cnf'
    Set-Content $StagingCnfPath '[mysqld]'
    Set-Content $script:TestIniPath '[mysqld]'
    $script:ExitCode = 0
    Invoke-ApplyConfig -Interactive $false -RestartService $false
    Assert-Code 11 $script:ExitCode 'apply rejects missing include before modification'
    $mysql_cnf_dir = Join-Path $temp 'absent'
    $script:ExitCode = 0
    Invoke-ApplyConfig -Interactive $false -RestartService $false
    Assert-Code 3 $script:ExitCode 'apply rejects missing directory'
    # Execute the actual rollback branch without entering the Windows bootstrap.
    $rollbackBranch = $ast.Find({ param($node)
        $node -is [System.Management.Automation.Language.IfStatementAst] -and
        $node.Clauses[0].Item1.Extent.Text -eq '$Rollback'
    }, $true)
    if (-not $rollbackBranch) { throw 'Missing rollback branch' }
    $script:ExitCode = 0
    & (ConvertTo-TestScriptBlock $rollbackBranch.Clauses[0].Item2 -Body)
    Assert-Code 3 $script:ExitCode 'rollback rejects missing directory'
    Remove-Item $StagingCnfPath
    $script:ExitCode = 0
    Invoke-ApplyConfig -Interactive $false -RestartService $false
    Assert-Code 1 $script:ExitCode 'missing recommendation retains code 1'
    $script:TestIniPath = $null
    Assert-Code 0 (Check-MySQLConfigInclude) 'undiscovered my.ini follows Linux behavior'
    Write-Host 'PASS: Windows configurer directory and include exit codes'
}

function Test-LegacyEntryPoint {
    param($temp)
    Copy-Item (Join-Path $root 'mysqlconfigurer.ps1') $temp
    $wrapper = Join-Path $temp 'mysqlconfigurer.ps1'
    $source = Get-Content (Join-Path $root 'releem.ps1') -Raw
    $start = $source.IndexOf('[CmdletBinding()]')
    $end = $source.IndexOf('Set-StrictMode')
    $stub = $source.Substring($start, $end - $start) + @'
if (-not $Apply -or $NonInteractive -or -not $NoRestart -or $ApplyMode -ne 'dynamic' -or $ApiKey -cne 'test key with spaces') { exit 99 }
exit 11
'@
    Set-Content (Join-Path $temp 'releem.ps1') $stub
    $driver = Join-Path $temp 'invoke.ps1'
    Set-Content $driver 'param($Wrapper); & $Wrapper -a -Automatic:$false -NoRestart -ApplyMode dynamic -k "test key with spaces"; exit $LASTEXITCODE'
    Invoke-TestScript $driver -Arguments @($wrapper) -ExpectedExit 11
    Set-Content (Join-Path $temp 'releem.ps1') ($source.Substring($start, $end - $start) + 'if (-not $Apply -or -not $NonInteractive -or -not $ForceRestart) { exit 99 }; exit 0')
    Invoke-TestScript $wrapper -Arguments @('-Apply', '-NonInteractive', '-ForceRestart')
    Set-Content (Join-Path $temp 'releem.ps1') ($source.Substring($start, $end - $start) + 'if (-not $Update) { exit 99 }; exit 0')
    Invoke-TestScript $wrapper -Arguments @('-u')
    Remove-Item (Join-Path $temp 'releem.ps1')
    Invoke-TestScript $wrapper -ExpectedExit 1 -Pattern 'releem\.ps1 is\s+missing'
    Write-Host 'PASS: legacy Windows wrapper forwards arguments and exit codes'
}

function Test-ConfigurerUpdate {
    param($temp)
    $programDir = Join-Path $temp 'program with spaces'
    $dataDir = Join-Path $temp 'data'
    New-Item -ItemType Directory $programDir, $dataDir | Out-Null
    $AgentBinaryPath = Join-Path $programDir 'agent-stub.ps1'
    $InstallerScriptPath = Join-Path $programDir 'install.ps1'
    $ConfigurerScriptPath = Join-Path $programDir 'releem.ps1'
    $LegacyConfigurerScriptPath = Join-Path $programDir 'mysqlconfigurer.ps1'
    $AgentDownloadUrl = 'https://example.test/releem-agent.exe'
    $InstallerUrl = 'https://example.test/install.ps1'
    $ReleemAgentScriptUrl = 'https://example.test/releem.ps1'
    $ReleemLegacyScriptUrl = 'https://example.test/mysqlconfigurer.ps1'

    # Use the actual command dispatcher; relocate its Windows bootstrap so this
    # test can run without administrator rights and also under PowerShell on Linux.
    $canonicalSource = Get-Content (Join-Path $root 'releem.ps1') -Raw
    $start = $canonicalSource.IndexOf('$currentPrincipal = ')
    $end = $canonicalSource.IndexOf('# Logging setup:', $start)
    if ($start -lt 0 -or $end -lt 0) { throw 'Cannot locate the Windows privilege bootstrap' }
    $canonicalSource = $canonicalSource.Remove($start, $end - $start)
    $canonicalSource = $canonicalSource.Replace('C:\ProgramData\ReleemAgent', $dataDir).Replace('C:\Program Files\ReleemAgent', $programDir)
    $legacySource = Get-Content (Join-Path $root 'mysqlconfigurer.ps1') -Raw
    $installerSource = Get-Content (Join-Path $root 'install.ps1') -Raw
    $installer = Get-ConfigurerAst install.ps1
    $update = $installer.Find({ param($node)
        $node -is [Management.Automation.Language.IfStatementAst] -and
        $node.Clauses[0].Item1.Extent.Text -eq '$u'
    }, $true)
    if (-not $update) { throw 'Missing installer update branch' }
    function Write-Log { param($Message) $script:messages += $Message }
    function Stop-ReleemAgentForReplacement { param($Reason) }
    function Get-Service { param($Name, $ErrorAction) return [pscustomobject]@{ Status = 'Running' } }
    function Invoke-WebRequest { param($Uri, $OutFile, [switch]$UseBasicParsing)
        $script:downloads += $Uri
        if ($Uri -eq $script:failDownloadUri) { throw "Simulated download failure: $Uri" }
        switch ($Uri) {
            $AgentDownloadUrl { Set-Content $OutFile 'exit 0' }
            $InstallerUrl { Set-Content $OutFile $installerSource }
            $ReleemAgentScriptUrl { Set-Content $OutFile $canonicalSource }
            $ReleemLegacyScriptUrl { Set-Content $OutFile $legacySource }
            default { throw "Unexpected download: $Uri" }
        }
    }
    $script:failDownloadUri = ''
    $script:messages = @()
    foreach ($scenario in @('legacy-only', 'both-outdated')) {
        Remove-Item $ConfigurerScriptPath -Force -ErrorAction SilentlyContinue
        Set-Content $LegacyConfigurerScriptPath 'exit 97'
        if ($scenario -eq 'both-outdated') { Set-Content $ConfigurerScriptPath 'exit 98' }
        $script:downloads = @()
        $script:MainExitCode = 99
        & (ConvertTo-TestScriptBlock $update.Clauses[0].Item2 -Body)
        if ($script:MainExitCode -ne 0) { throw "Update failed: $scenario / $script:MainExitCode" }
        foreach ($uri in @($ReleemAgentScriptUrl, $ReleemLegacyScriptUrl)) {
            if (@($script:downloads | Where-Object { $_ -eq $uri }).Count -ne 1) { throw "Update must download $uri once" }
        }
        foreach ($entryPoint in @($ConfigurerScriptPath, $LegacyConfigurerScriptPath)) {
            if (-not (Test-Path $entryPoint -PathType Leaf)) { throw "Update did not install $entryPoint" }
            Invoke-TestScript $entryPoint -Pattern 'Usage: releem.ps1'
            Invoke-TestScript $entryPoint -Arguments @('-NonInteractive') -ExpectedExit 1 -Pattern '-NonInteractive can only be used together with -Apply'
        }
        Write-Host "PASS: $scenario update installs working canonical and legacy entry points"
    }
    # Each required script must fail the real update before service start or success.
    foreach ($uri in @($ReleemAgentScriptUrl, $ReleemLegacyScriptUrl)) {
        $script:failDownloadUri = $uri
        $script:messages = @()
        $script:MainExitCode = 99
        & (ConvertTo-TestScriptBlock $update.Clauses[0].Item2 -Body)
        if ($script:MainExitCode -ne 1) { throw "Download failure was reported as success: $uri" }
        if ($script:messages -contains 'Starting Releem Agent service...' -or
            $script:messages -contains 'Releem Agent updated successfully.') {
            throw "Update continued after required script download failure: $uri"
        }
        Write-Host "PASS: update stops on failed download of $([IO.Path]::GetFileName($uri))"
    }

    # Exercise the fresh-install download block and ensure its catch exits dispatch.
    $downloadBlocks = $installer.FindAll({ param($node)
        $node -is [Management.Automation.Language.TryStatementAst] -and
        $node.Body.Statements[0].Extent.Text.StartsWith('Invoke-WebRequest -Uri $ReleemAgentScriptUrl')
    }, $true)
    if ($downloadBlocks.Count -ne 2) { throw 'Expected fresh-install and update download blocks' }
    $freshDownload = $downloadBlocks[1].Extent.Text
    $script:failDownloadUri = ''
    $script:downloads = @()
    & (ConvertTo-TestScriptBlock $downloadBlocks[1])
    if (($script:downloads -join ',') -ne (@($ReleemAgentScriptUrl, $ReleemLegacyScriptUrl) -join ',')) {
        throw 'Fresh installation must download both required scripts once, in order'
    }
    Write-Host 'PASS: fresh installation downloads both entry points'
    foreach ($uri in @($ReleemAgentScriptUrl, $ReleemLegacyScriptUrl)) {
        $script:failDownloadUri = $uri
        $script:MainExitCode = 99
        $script:continued = $false
        & ([scriptblock]::Create($freshDownload + "`n" + '$script:continued = $true'))
        if ($script:MainExitCode -ne 1 -or $script:continued) {
            throw "Fresh installation continued after required script download failure: $uri"
        }
        Write-Host "PASS: fresh installation stops on failed download of $([IO.Path]::GetFileName($uri))"
    }
}

function Test-InstallerServiceDetection {
    param($temp)
    $ast = Get-ConfigurerAst install.ps1
    $loop = $ast.Find({
        param($node)
        $node -is [Management.Automation.Language.ForEachStatementAst] -and
            $node.Extent.Text -match 'Get-Service -Name' -and
            $node.Extent.Text -match 'MysqlServiceName'
    }, $true)
    if (-not $loop) { throw 'Installer service detection loop was not found' }
    $detectService = ConvertTo-TestScriptBlock $loop
    $assignment = $ast.Find({
        param($node)
        $node -is [Management.Automation.Language.AssignmentStatementAst] -and
            $node.Left.Extent.Text -eq '$restartCmd'
    }, $true)
    if (-not $assignment) { throw 'Installer restart command assignment was not found' }
    $buildRestartCommand = ConvertTo-TestScriptBlock $assignment
    function Write-Log { param($Message) }
    function Get-Service {
        param($Name, $ErrorAction)
        if ($Name -eq $availableService) { return [pscustomobject]@{ Name = $Name } }
    }
    foreach ($availableService in @('MySQL90', 'MySQL97', 'MySQL267', 'MySQL84', 'MySQL80', 'MySQL57', 'MySQL56', 'MySQL', 'mariadb', 'mysqld', 'unknown')) {
        $MysqlServiceName = $null
        . $detectService
        . $buildRestartCommand
        if ($availableService -eq 'unknown') {
            Assert-Code '' $restartCmd 'undetected service leaves restart command empty'
        } else {
            Assert-Code $availableService $MysqlServiceName 'detected MySQL service'
            Assert-Code "net stop $availableService && net start $availableService" $restartCmd 'generated restart command'
        }
        Write-Host "PASS: installer service detection: $availableService"
    }
}

# Parse each source only once and isolate each scenario's mocks and files.
foreach ($name in @('install.ps1', 'releem.ps1', 'mysqlconfigurer.ps1')) { $null = Get-ConfigurerAst $name }
foreach ($case in @(
    @{ Name = 'ExitCodes'; Function = 'Test-ConfigurerExitCodes' },
    @{ Name = 'Legacy'; Function = 'Test-LegacyEntryPoint' },
    @{ Name = 'Update'; Function = 'Test-ConfigurerUpdate' },
    @{ Name = 'InstallServices'; Function = 'Test-InstallerServiceDetection' }
)) {
    if ($Check -ne 'All' -and $Check -ne $case.Name) { continue }
    $temp = Join-Path ([IO.Path]::GetTempPath()) ([guid]::NewGuid().ToString())
    New-Item -ItemType Directory $temp | Out-Null
    try {
        & $case.Function $temp
    } finally {
        Remove-Item $temp -Recurse -Force
    }
}
