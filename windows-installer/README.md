# Releem Agent Windows Installer

Windows MSI installer for Releem Agent built with WiX Toolset.

## Prerequisites

- **Windows 10/11** or **Windows Server 2016+**
- **.NET SDK 6.0+** - [Download](https://dotnet.microsoft.com/download)
- **WiX Toolset 3.11+** - [Download](https://github.com/wixtoolset/wix3/releases)

### Installing WiX Toolset

1. Download WiX Toolset 3.11 from the releases page
2. Run the installer
3. Ensure `candle.exe` and `light.exe` are in your PATH

## Building

### Using Command Prompt

```batch
cd windows-installer
build.bat
```

### Using PowerShell

```powershell
cd windows-installer
.\build.ps1
```

### Build with custom version

```powershell
.\build.ps1 -Version "1.2.3"
```

## Output

After a successful build, the MSI installer will be located at:

```
bin\releem-agent-setup.msi
```

## Project Structure

```
windows-installer/
├── Product.wxs              # Main WiX installer definition
├── UI.wxs                   # Custom UI dialogs
├── CustomActions/
│   ├── CustomActions.cs     # C# custom actions
│   └── CustomActions.csproj # .NET project file
├── build.bat                # Build script (cmd)
├── build.ps1                # Build script (PowerShell)
└── README.md                # This file
```

## Installation

### GUI Installation

1. Double-click `releem-agent-setup.msi`
2. Follow the wizard:
   - Enter your Releem API key
   - Configure MySQL connection details
   - Enable/disable Query Optimization
3. Click Install

### Silent Installation

```batch
msiexec /i releem-agent-setup.msi /qn ^
  RELEEM_APIKEY="your-api-key" ^
  MYSQL_HOST="127.0.0.1" ^
  MYSQL_PORT="3306" ^
  MYSQL_USER="releem" ^
  MYSQL_PASSWORD="your-password" ^
  QUERY_OPTIMIZATION="true"
```

### Installation with Logging

```batch
msiexec /i releem-agent-setup.msi /l*v install.log
```

## Uninstallation

### Via Control Panel

1. Open **Settings** > **Apps** > **Apps & features**
2. Find "Releem Agent"
3. Click **Uninstall**

### Via Command Line

```batch
msiexec /x releem-agent-setup.msi /qn
```

## File Locations

After installation:

| Item | Location |
|------|----------|
| Executable | `C:\Program Files\ReleemAgent\releem-agent.exe` |
| Configuration | `C:\ProgramData\ReleemAgent\releem.conf` |
| Additional configs | `C:\ProgramData\ReleemAgent\conf.d\` |

## Service Management

The installer automatically registers and starts the Releem Agent Windows service.

```batch
# Check service status
sc query releem-agent

# Start service
net start releem-agent

# Stop service
net stop releem-agent

# Or use the agent directly
"C:\Program Files\ReleemAgent\releem-agent.exe" status
"C:\Program Files\ReleemAgent\releem-agent.exe" start
"C:\Program Files\ReleemAgent\releem-agent.exe" stop
```

## Troubleshooting

### Build Errors

1. Ensure WiX Toolset is installed and in PATH
2. Ensure .NET SDK is installed
3. Run `dotnet restore` in the CustomActions folder

### Installation Errors

1. Run with logging: `msiexec /i setup.msi /l*v log.txt`
2. Check the log file for errors
3. Ensure you have administrator privileges

### Service Won't Start

1. Check the configuration file exists and is valid
2. Verify MySQL credentials are correct
3. Check Windows Event Viewer for errors

## Links

- [Releem Dashboard](https://releem.com/dashboard)
- [Documentation](https://docs.releem.com)
- [Get API Key](https://releem.com)
