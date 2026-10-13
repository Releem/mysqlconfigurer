# Offline update regression: execute the installer's real -u branch with mocked
# downloads and service control, then invoke both updated entry points.
param([string]$WindowsScriptDir = (Join-Path $PSScriptRoot '../../windows'))

$ErrorActionPreference = 'Stop'
$root = $WindowsScriptDir
$engine = (Get-Process -Id $PID).Path
$temp = Join-Path ([IO.Path]::GetTempPath()) ([guid]::NewGuid().ToString())
New-Item -ItemType Directory $temp | Out-Null
try {
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
    $tokens = $null; $parseErrors = $null
    $installer = [Management.Automation.Language.Parser]::ParseInput($installerSource, [ref]$tokens, [ref]$parseErrors)
    if ($parseErrors.Count) { throw ($parseErrors | Out-String) }
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
        $body = $update.Clauses[0].Item2.Extent.Text
        & ([scriptblock]::Create($body.Substring(1, $body.Length - 2)))
        if ($script:MainExitCode -ne 0) { throw "Update failed: $scenario / $script:MainExitCode" }
        foreach ($uri in @($ReleemAgentScriptUrl, $ReleemLegacyScriptUrl)) {
            if (@($script:downloads | Where-Object { $_ -eq $uri }).Count -ne 1) { throw "Update must download $uri once" }
        }
        foreach ($entryPoint in @($ConfigurerScriptPath, $LegacyConfigurerScriptPath)) {
            if (-not (Test-Path $entryPoint -PathType Leaf)) { throw "Update did not install $entryPoint" }
            $output = & $engine -NoProfile -File $entryPoint 2>&1 | Out-String
            if ($LASTEXITCODE -ne 0 -or $output -notmatch 'Usage: releem.ps1') {
                throw "Updated entry point cannot reach canonical help: $scenario / $entryPoint / $LASTEXITCODE / $output"
            }
            $output = & $engine -NoProfile -File $entryPoint -NonInteractive 2>&1 | Out-String
            if ($LASTEXITCODE -ne 1 -or $output -notmatch '-NonInteractive can only be used together with -Apply') {
                throw "Updated entry point did not forward validation error: $scenario / $entryPoint / $LASTEXITCODE"
            }
        }
        Write-Host "PASS: $scenario update installs working canonical and legacy entry points"
    }
    # Each required script must fail the real update before service start or success.
    foreach ($uri in @($ReleemAgentScriptUrl, $ReleemLegacyScriptUrl)) {
        $script:failDownloadUri = $uri
        $script:messages = @()
        $script:MainExitCode = 99
        & ([scriptblock]::Create($body.Substring(1, $body.Length - 2)))
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
} finally {
    Remove-Item $temp -Recurse -Force
}
