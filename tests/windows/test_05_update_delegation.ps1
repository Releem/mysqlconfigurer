# Test 5: releem.ps1 -Update delegates update work to install.ps1 -u.

param()

. "$PSScriptRoot\helpers.ps1"

$ConfigurerUnderTest = if ($env:CONFIGURER_SCRIPT) { $env:CONFIGURER_SCRIPT } else { throw "CONFIGURER_SCRIPT must be set" }
$LegacyUnderTest = Join-Path (Split-Path $ConfigurerUnderTest) 'mysqlconfigurer.ps1'
if (-not (Test-Path $LegacyUnderTest)) { throw "Legacy wrapper payload is missing: $LegacyUnderTest" }

Write-Info "=== Test 5: releem.ps1 delegates update to install.ps1 -u ==="

& powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$PSScriptRoot\test_update_script_compatibility.ps1" -WindowsScriptDir (Split-Path $ConfigurerUnderTest)
Assert-Zero "Real installer update branch leaves both entry points working" $LASTEXITCODE

$programDir = "C:\Program Files\ReleemAgent"
$dataDir    = "C:\ProgramData\ReleemAgent"
$markerPath = Join-Path $dataDir "update-delegation-marker.txt"
$configPath = Join-Path $dataDir "releem.conf"
$updatedConfigurer = Join-Path $programDir "releem.ps1"
$updatedLegacy = Join-Path $programDir "mysqlconfigurer.ps1"

Remove-ReleemAgent

New-Item -ItemType Directory -Path $programDir -Force | Out-Null
New-Item -ItemType Directory -Path $dataDir -Force | Out-Null
# Simulate a legacy installation: no canonical entry point and an outdated wrapper.
Remove-Item $updatedConfigurer -Force -ErrorAction SilentlyContinue
Set-Content $updatedLegacy 'exit 97'

Remove-Item -Path $markerPath -Force -ErrorAction SilentlyContinue

@'
apikey="delegation-test-key"
'@ | Set-Content -Path $configPath -Encoding UTF8

function Get-FreeTcpPort {
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    $listener.Start()
    try {
        return ([System.Net.IPEndPoint]$listener.LocalEndpoint).Port
    } finally {
        $listener.Stop()
    }
}

$port = Get-FreeTcpPort
$stubInstaller = @"
param([switch]`$u, [switch]`$Uninstall)
Set-Content -Path '$markerPath' -Value ('u=' + `$u.ToString() + ';api=' + `$env:RELEEM_API_KEY) -Encoding UTF8
if (-not `$u) { exit 9 }
Copy-Item -LiteralPath '$ConfigurerUnderTest' -Destination '$updatedConfigurer' -Force
Copy-Item -LiteralPath '$LegacyUnderTest' -Destination '$updatedLegacy' -Force
exit 0
"@

$serverJob = Start-Job -ArgumentList $port, $stubInstaller -ScriptBlock {
    param($ListenPort, $InstallerBody)

    $listener = [System.Net.HttpListener]::new()
    $listener.Prefixes.Add("http://127.0.0.1:$ListenPort/")
    $listener.Start()

    try {
        for ($i = 0; $i -lt 2; $i++) {
            $context  = $listener.GetContext()
            $response = $context.Response

            switch ($context.Request.RawUrl) {
                '/v2/current_version_agent' {
                    $body = '99.0.0' # Always newer than the locally tested script.
                    $response.ContentType = 'text/plain'
                    $response.StatusCode = 200
                }
                '/v2/install.ps1' {
                    $body = $InstallerBody
                    $response.ContentType = 'text/plain'
                    $response.StatusCode = 200
                }
                default {
                    $body = 'not found'
                    $response.ContentType = 'text/plain'
                    $response.StatusCode = 404
                }
            }

            $bytes = [System.Text.Encoding]::UTF8.GetBytes($body)
            $response.ContentLength64 = $bytes.Length
            $response.OutputStream.Write($bytes, 0, $bytes.Length)
            $response.OutputStream.Close()
        }
    } finally {
        $listener.Stop()
        $listener.Close()
    }
}

$env:RELEEM_CURRENT_VERSION_URL  = "http://127.0.0.1:$port/v2/current_version_agent"
$env:RELEEM_INSTALLER_SCRIPT_URL = "http://127.0.0.1:$port/v2/install.ps1"

try {
    & powershell.exe -ExecutionPolicy Bypass -File $ConfigurerUnderTest -Update
    $updateExit = $LASTEXITCODE

    Assert-Zero "releem.ps1 -Update exited successfully" $updateExit
    Assert-FileExists "Delegated installer marker created" $markerPath
    Assert-FileContains "Delegated installer received -u switch" $markerPath 'u=True'
    Assert-FileContains "Delegated installer inherited RELEEM_API_KEY" $markerPath 'api=delegation-test-key'

    Assert-FileExists "Update installed the canonical entry point" $updatedConfigurer
    Assert-FileExists "Update installed the legacy entry point" $updatedLegacy
    Assert-Equal "Canonical script matches the update payload" (Get-FileHash $ConfigurerUnderTest).Hash (Get-FileHash $updatedConfigurer).Hash
    Assert-Equal "Legacy wrapper matches the update payload" (Get-FileHash $LegacyUnderTest).Hash (Get-FileHash $updatedLegacy).Hash

    # Help dispatch exercises the actual updated scripts without touching MySQL.
    # No API key is needed, so log upload stays offline for these invocations.
    'apikey=""' | Set-Content -Path $configPath -Encoding UTF8
    foreach ($entryPoint in @($updatedConfigurer, $updatedLegacy)) {
        $output = & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $entryPoint 2>&1 | Out-String
        Assert-Zero "Updated $([IO.Path]::GetFileName($entryPoint)) runs successfully" $LASTEXITCODE
        if ($output -match 'Usage: releem.ps1') {
            Write-Pass "Updated $([IO.Path]::GetFileName($entryPoint)) reaches the canonical command dispatcher"
        } else {
            Write-Fail "Updated $([IO.Path]::GetFileName($entryPoint)) did not produce canonical usage"
        }
    }

    $jobResult = Wait-Job -Job $serverJob -Timeout 10
    if (-not $jobResult) {
        Write-Fail "Update test HTTP server did not finish in time"
    } elseif ($serverJob.State -ne 'Completed') {
        Write-Fail "Update test HTTP server exited unexpectedly with state '$($serverJob.State)'"
    } else {
        Write-Pass "Update test HTTP server served expected requests"
    }
} finally {
    Remove-Job -Job $serverJob -Force -ErrorAction SilentlyContinue
    Remove-Item Env:RELEEM_CURRENT_VERSION_URL -ErrorAction SilentlyContinue
    Remove-Item Env:RELEEM_INSTALLER_SCRIPT_URL -ErrorAction SilentlyContinue
}

Show-Summary "Test 5: releem.ps1 update delegation"
