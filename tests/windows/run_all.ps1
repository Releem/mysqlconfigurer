#Requires -RunAsAdministrator
# Run all Releem agent Windows tests in sequence.
#
# Usage:
#   .\run_all.ps1 [-Test 1|2|3|4|all]
#
# Required env vars:
#   RELEEM_API_KEY, MYSQL_ROOT_PASSWORD, OS_VERSION

param(
    [string]$Test = "all"
)

$ErrorActionPreference = "Stop"
$ScriptDir = $PSScriptRoot

$env:RELEEM_API_KEY      = if ($env:RELEEM_API_KEY)      { $env:RELEEM_API_KEY }      else { throw "RELEEM_API_KEY must be set" }
$env:MYSQL_ROOT_PASSWORD = if ($env:MYSQL_ROOT_PASSWORD) { $env:MYSQL_ROOT_PASSWORD } else { throw "MYSQL_ROOT_PASSWORD must be set" }
if (-not $env:OS_VERSION) { throw "OS_VERSION must be set" }

# Copy scripts to staging area
New-Item -ItemType Directory -Path "C:\releem_tests" -Force | Out-Null

if ($env:INSTALL_SCRIPT -and (Test-Path $env:INSTALL_SCRIPT)) {
    $installDst = "C:\releem_tests\install.ps1"
    if ([System.IO.Path]::GetFullPath($env:INSTALL_SCRIPT) -ne [System.IO.Path]::GetFullPath($installDst)) {
        Copy-Item $env:INSTALL_SCRIPT $installDst -Force
    }
} elseif (Test-Path "C:\install.ps1") {
    Copy-Item "C:\install.ps1" "C:\releem_tests\install.ps1" -Force
}
if ($env:CONFIGURER_SCRIPT -and (Test-Path $env:CONFIGURER_SCRIPT)) {
    $configurerDst = "C:\releem_tests\mysqlconfigurer.ps1"
    if ([System.IO.Path]::GetFullPath($env:CONFIGURER_SCRIPT) -ne [System.IO.Path]::GetFullPath($configurerDst)) {
        Copy-Item $env:CONFIGURER_SCRIPT $configurerDst -Force
    }
} elseif (Test-Path "C:\mysqlconfigurer.ps1") {
    Copy-Item "C:\mysqlconfigurer.ps1" "C:\releem_tests\mysqlconfigurer.ps1" -Force
}

$env:INSTALL_SCRIPT    = "C:\releem_tests\install.ps1"
$env:CONFIGURER_SCRIPT = "C:\releem_tests\mysqlconfigurer.ps1"

$OverallPassed = 0
$OverallFailed = 0

function Invoke-Test {
    param([string]$Num, [string]$Script, [string]$Name)
    if ($Test -ne "all" -and $Test -ne $Num) { return }

    Write-Host ""
    Write-Host "######################################"
    Write-Host "# Running $Name"
    Write-Host "######################################"

    & powershell.exe -ExecutionPolicy Bypass -File "$ScriptDir\$Script"
    if ($LASTEXITCODE -eq 0) {
        Write-Host "[SUITE] $Name`: PASSED"
        $script:OverallPassed++
    } else {
        Write-Host "[SUITE] $Name`: FAILED"
        $script:OverallFailed++
    }
}

Invoke-Test "1" "test_01_install_auto.ps1"          "Test 1: Fresh install (auto user creation)"
Invoke-Test "2" "test_02_install_existing_user.ps1" "Test 2: Install with pre-existing user"

# Re-run test 1 to restore state for tests 3 and 4 (if running full suite)
if ($Test -eq "all") {
    Write-Host ""
    Write-Host "[SUITE] Re-running Test 1 to restore state for Tests 3 and 4..."
    & powershell.exe -ExecutionPolicy Bypass -File "$ScriptDir\test_01_install_auto.ps1" | Out-Null
}

Invoke-Test "3" "test_03_apply_config.ps1"    "Test 3: Apply configuration"
Invoke-Test "4" "test_04_rollback_config.ps1" "Test 4: Rollback configuration"

Write-Host ""
Write-Host "========================================="
Write-Host "OVERALL TEST SUITE RESULTS"
Write-Host "  PASSED: $OverallPassed"
Write-Host "  FAILED: $OverallFailed"
Write-Host "========================================="

if ($OverallFailed -gt 0) { exit 1 }
exit 0
