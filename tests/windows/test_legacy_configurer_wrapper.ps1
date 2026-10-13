# Offline compatibility checks; no database or Windows services required.
$ErrorActionPreference = 'Stop'
$root = Join-Path $PSScriptRoot '../../windows'
$engine = (Get-Process -Id $PID).Path
$temp = Join-Path ([IO.Path]::GetTempPath()) ([guid]::NewGuid().ToString())
New-Item -ItemType Directory $temp | Out-Null
try {
    foreach ($name in @('install.ps1', 'releem.ps1', 'mysqlconfigurer.ps1')) {
        $tokens = $null; $errors = $null
        $null = [Management.Automation.Language.Parser]::ParseFile((Join-Path $root $name), [ref]$tokens, [ref]$errors)
        if ($errors.Count) { throw ($errors | Out-String) }
    }
    # Execute both installer download blocks with network calls mocked.
    $tokens = $null; $errors = $null
    $installer = [Management.Automation.Language.Parser]::ParseFile((Join-Path $root 'install.ps1'), [ref]$tokens, [ref]$errors)
    $ReleemAgentScriptUrl = 'https://example.test/releem.ps1'
    $ReleemLegacyScriptUrl = 'https://example.test/mysqlconfigurer.ps1'
    $ConfigurerScriptPath = Join-Path $temp 'releem.ps1'
    $LegacyConfigurerScriptPath = Join-Path $temp 'mysqlconfigurer.ps1'
    function Write-Log { param($Message) }
    function Invoke-WebRequest { param($Uri, $OutFile, [switch]$UseBasicParsing)
        $script:downloads += "$Uri -> $OutFile"
    }
    $blocks = $installer.FindAll({ param($node)
        $node -is [Management.Automation.Language.TryStatementAst] -and
        $node.Body.Statements[0].Extent.Text.StartsWith('Invoke-WebRequest -Uri $ReleemAgentScriptUrl')
    }, $true)
    if ($blocks.Count -ne 2) { throw 'Expected fresh-install and update download blocks' }
    foreach ($block in $blocks) {
        $script:downloads = @()
        & ([scriptblock]::Create($block.Body.Extent.Text.Trim().TrimStart('{').TrimEnd('}')))
        if ($script:downloads.Count -ne 2 -or
            $script:downloads[0] -ne "$ReleemAgentScriptUrl -> $ConfigurerScriptPath" -or
            $script:downloads[1] -ne "$ReleemLegacyScriptUrl -> $LegacyConfigurerScriptPath") {
            throw 'Installer must download both Windows scripts'
        }
    }
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
    & $engine -NoProfile -File $driver $wrapper
    if ($LASTEXITCODE -ne 11) { throw "Argument/exit-code forwarding failed: $LASTEXITCODE" }
    Set-Content (Join-Path $temp 'releem.ps1') ($source.Substring($start, $end - $start) + 'if (-not $Update) { exit 99 }; exit 0')
    & $engine -NoProfile -File $wrapper -u
    if ($LASTEXITCODE -ne 0) { throw 'Update alias forwarding failed' }
    Remove-Item (Join-Path $temp 'releem.ps1')
    & $engine -NoProfile -File $wrapper 2>$null
    if ($LASTEXITCODE -ne 1) { throw 'Missing implementation must fail with code 1' }
    Write-Host 'PASS: legacy Windows wrapper forwards arguments and exit codes'
} finally {
    Remove-Item $temp -Recurse -Force
}
