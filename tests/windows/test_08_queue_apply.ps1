# Test 8: mysqlconfigurer.ps1 -QueueApply delegates apply scheduling to releem-agent --task=apply_config.

param()

. "$PSScriptRoot\helpers.ps1"

$markerPath    = "C:\releem_tests\queue-apply-marker.txt"
$fakeAgentPath = "C:\releem_tests\fake-releem-agent.cmd"

Write-Info "=== Test 8: Queue apply via releem-agent task ==="

Remove-Item -Path $markerPath -Force -ErrorAction SilentlyContinue

@"
@echo off
> "$markerPath" echo %*
exit /b 0
"@ | Set-Content -Path $fakeAgentPath -Encoding ASCII

$env:RELEEM_AGENT_BINARY_PATH = $fakeAgentPath

try {
    & powershell.exe -ExecutionPolicy Bypass -File $ConfigurerScript -QueueApply
    $queueExit = $LASTEXITCODE

    Assert-Zero "mysqlconfigurer.ps1 -QueueApply exited successfully" $queueExit
    Assert-FileExists "Queue apply marker created" $markerPath
    Assert-FileContains "Queue apply invoked releem-agent with apply task" $markerPath '--task=apply_config'
} finally {
    Remove-Item Env:RELEEM_AGENT_BINARY_PATH -ErrorAction SilentlyContinue
}

Show-Summary "Test 8: Queue apply via releem-agent task"
