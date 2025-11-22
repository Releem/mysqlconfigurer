# Releem Agent Windows Installer Build Script (PowerShell)

param(
    [string]$Version = "1.0.0",
    [switch]$SkipDownload
)

$ErrorActionPreference = "Stop"

Write-Host "============================================" -ForegroundColor Cyan
Write-Host "Releem Agent Windows Installer Build Script" -ForegroundColor Cyan
Write-Host "============================================" -ForegroundColor Cyan
Write-Host ""

# Check for required tools
try {
    $null = Get-Command dotnet -ErrorAction Stop
} catch {
    Write-Host "ERROR: .NET SDK not found. Please install .NET SDK 6.0 or later." -ForegroundColor Red
    exit 1
}

try {
    $null = Get-Command candle -ErrorAction Stop
} catch {
    Write-Host "ERROR: WiX Toolset not found. Please install WiX Toolset 3.11+" -ForegroundColor Red
    Write-Host "Download from: https://github.com/wixtoolset/wix3/releases" -ForegroundColor Yellow
    exit 1
}

# Create output directories
if (-not (Test-Path "obj")) { New-Item -ItemType Directory -Path "obj" | Out-Null }
if (-not (Test-Path "bin")) { New-Item -ItemType Directory -Path "bin" | Out-Null }

# Step 1: Build Custom Actions
Write-Host "[1/4] Building Custom Actions..." -ForegroundColor Green
Push-Location CustomActions
try {
    dotnet restore
    if ($LASTEXITCODE -ne 0) { throw "Failed to restore NuGet packages" }

    dotnet build -c Release
    if ($LASTEXITCODE -ne 0) { throw "Failed to build custom actions" }
} finally {
    Pop-Location
}
Write-Host "Custom Actions built successfully." -ForegroundColor Green
Write-Host ""

# Step 2: Skip exe download (will download during installation)
Write-Host "[2/4] Skipping exe download (will download during installation)..." -ForegroundColor Green
Write-Host ""

# Step 3: Compile WiX sources
Write-Host "[3/4] Compiling WiX sources..." -ForegroundColor Green
$candleArgs = @(
    "Product.wxs",
    "UI.wxs",
    "-ext", "WixUtilExtension",
    "-ext", "WixUIExtension",
    "-out", "obj\",
    "-arch", "x64",
    "-dVersion=$Version"
)
& candle.exe @candleArgs
if ($LASTEXITCODE -ne 0) {
    Write-Host "ERROR: WiX compilation failed" -ForegroundColor Red
    exit 1
}
Write-Host "WiX compilation successful." -ForegroundColor Green
Write-Host ""

# Step 4: Link MSI
Write-Host "[4/4] Linking MSI..." -ForegroundColor Green
$lightArgs = @(
    "obj\Product.wixobj",
    "obj\UI.wixobj",
    "-ext", "WixUtilExtension",
    "-ext", "WixUIExtension",
    "-b", "ca=CustomActions\bin\Release\net48",
    "-out", "bin\releem-agent-setup.msi",
    "-sice:ICE61",
    "-sice:ICE71"
)
& light.exe @lightArgs
if ($LASTEXITCODE -ne 0) {
    Write-Host "ERROR: MSI linking failed" -ForegroundColor Red
    exit 1
}

Write-Host ""
Write-Host "============================================" -ForegroundColor Cyan
Write-Host "Build completed successfully!" -ForegroundColor Green
Write-Host "Output: bin\releem-agent-setup.msi" -ForegroundColor Green
Write-Host "============================================" -ForegroundColor Cyan
