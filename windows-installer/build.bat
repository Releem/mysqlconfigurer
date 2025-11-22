@echo off
setlocal enabledelayedexpansion

echo ============================================
echo Releem Agent Windows Installer Build Script
echo ============================================
echo.

:: Check for required tools
where dotnet >nul 2>&1
if %errorlevel% neq 0 (
    echo ERROR: .NET SDK not found. Please install .NET SDK 6.0 or later.
    exit /b 1
)

where candle >nul 2>&1
if %errorlevel% neq 0 (
    echo ERROR: WiX Toolset not found. Please install WiX Toolset 3.11+
    echo Download from: https://github.com/wixtoolset/wix3/releases
    exit /b 1
)

:: Create output directories
if not exist "obj" mkdir obj
if not exist "bin" mkdir bin

echo [1/4] Building Custom Actions...
cd CustomActions
dotnet restore
if %errorlevel% neq 0 (
    echo ERROR: Failed to restore NuGet packages
    cd ..
    exit /b 1
)

dotnet build -c Release
if %errorlevel% neq 0 (
    echo ERROR: Failed to build custom actions
    cd ..
    exit /b 1
)
cd ..
echo Custom Actions built successfully.
echo.

:: Note: releem-agent.exe will be downloaded during installation
echo [2/4] Skipping exe download (will download during installation)...
echo.

echo [3/4] Compiling WiX sources...
candle.exe Product.wxs UI.wxs ^
    -ext WixUtilExtension ^
    -ext WixUIExtension ^
    -out obj\ ^
    -arch x64
if %errorlevel% neq 0 (
    echo ERROR: WiX compilation failed
    exit /b 1
)
echo WiX compilation successful.
echo.

echo [4/4] Linking MSI...
light.exe obj\Product.wixobj obj\UI.wixobj ^
    -ext WixUtilExtension ^
    -ext WixUIExtension ^
    -b ca=CustomActions\bin\Release\net48 ^
    -out bin\releem-agent-setup.msi ^
    -sice:ICE61
if %errorlevel% neq 0 (
    echo ERROR: MSI linking failed
    exit /b 1
)

echo.
echo ============================================
echo Build completed successfully!
echo Output: bin\releem-agent-setup.msi
echo ============================================

endlocal
