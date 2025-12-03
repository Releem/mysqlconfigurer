using System;
using System.Diagnostics;
using System.IO;
using System.Net;
using System.Text;
using WixToolset.Dtf.WindowsInstaller;

namespace ReleemCustomActions
{
    public class CustomActions
    {
        /// <summary>
        /// Run install.ps1 with parameters collected from MSI UI
        /// </summary>
        [CustomAction]
        public static ActionResult RunInstaller(Session session)
        {
            session.Log("Begin RunInstaller");

            try
            {
                // Parse custom action data
                var data = session.CustomActionData;
                string installDir = data["INSTALLDIR"];
                string apiKey = data["RELEEM_APIKEY"];
                string mysqlHost = data["MYSQL_HOST"];
                string mysqlPort = data["MYSQL_PORT"];
                string mysqlUser = data["MYSQL_USER"];
                string mysqlPassword = data["MYSQL_PASSWORD"];
                string queryOptimization = data["QUERY_OPTIMIZATION"];

                session.Log($"Install directory: {installDir}");
                session.Log($"MySQL Host: {mysqlHost}");
                session.Log($"MySQL Port: {mysqlPort}");
                session.Log($"MySQL User: {mysqlUser}");
                session.Log($"Query Optimization: {queryOptimization}");

                // Validate required parameters
                if (string.IsNullOrEmpty(apiKey))
                {
                    session.Log("ERROR: API key is required");
                    return ActionResult.Failure;
                }

                if (string.IsNullOrEmpty(installDir))
                {
                    session.Log("ERROR: Install directory is not set");
                    return ActionResult.Failure;
                }

                // Ensure install directory exists
                if (!Directory.Exists(installDir))
                {
                    Directory.CreateDirectory(installDir);
                    session.Log($"Created install directory: {installDir}");
                }

                // Download install.ps1 from S3
                string installScript = Path.Combine(installDir, "install.ps1");
                string downloadUrl = "https://releem.s3.amazonaws.com/v2/install.ps1";

                session.Log($"Downloading install.ps1 from {downloadUrl}");

                try
                {
                    ServicePointManager.SecurityProtocol = SecurityProtocolType.Tls12;
                    using (WebClient client = new WebClient())
                    {
                        client.DownloadFile(downloadUrl, installScript);
                    }

                    if (File.Exists(installScript))
                    {
                        FileInfo fileInfo = new FileInfo(installScript);
                        session.Log($"Successfully downloaded install.ps1 ({fileInfo.Length} bytes)");
                    }
                    else
                    {
                        session.Log("ERROR: install.ps1 download failed - file not found after download");
                        return ActionResult.Failure;
                    }
                }
                catch (Exception ex)
                {
                    session.Log($"ERROR downloading install.ps1: {ex.Message}");

                    using (Record record = new Record(0))
                    {
                        record.SetString(0, $"Failed to download installation script from S3.\n\nError: {ex.Message}\n\nPlease check your internet connection and try again.");
                        session.Message(InstallMessage.Error, record);
                    }

                    return ActionResult.Failure;
                }

                // Build PowerShell arguments
                var arguments = new StringBuilder();
                arguments.Append($"-ExecutionPolicy Bypass -NoProfile -WindowStyle Hidden -File \"{installScript}\" ");
                arguments.Append($"-ApiKey \"{apiKey}\" ");
                arguments.Append($"-MysqlHost \"{mysqlHost}\" ");
                arguments.Append($"-MysqlPort \"{mysqlPort}\" ");
                arguments.Append($"-MysqlUser \"{mysqlUser}\" ");

                if (!string.IsNullOrEmpty(mysqlPassword))
                {
                    arguments.Append($"-MysqlPassword \"{mysqlPassword}\" ");
                }

                arguments.Append($"-QueryOptimization ${queryOptimization.ToLower()} ");
                arguments.Append("-Silent");

                session.Log($"Executing PowerShell with arguments: {arguments.ToString().Replace(apiKey, "***").Replace(mysqlPassword ?? "", "***")}");

                // Start PowerShell process
                var processInfo = new ProcessStartInfo
                {
                    FileName = "powershell.exe",
                    Arguments = arguments.ToString(),
                    UseShellExecute = false,
                    RedirectStandardOutput = true,
                    RedirectStandardError = true,
                    CreateNoWindow = true
                };

                using (var process = Process.Start(processInfo))
                {
                    // Read output and errors
                    string output = process.StandardOutput.ReadToEnd();
                    string errors = process.StandardError.ReadToEnd();

                    process.WaitForExit();

                    // Log output
                    if (!string.IsNullOrEmpty(output))
                    {
                        session.Log($"PowerShell Output: {output}");
                    }

                    if (!string.IsNullOrEmpty(errors))
                    {
                        session.Log($"PowerShell Errors: {errors}");
                    }

                    session.Log($"PowerShell exit code: {process.ExitCode}");

                    if (process.ExitCode != 0)
                    {
                        session.Log($"ERROR: install.ps1 failed with exit code {process.ExitCode}");

                        // Create message for user
                        using (Record record = new Record(0))
                        {
                            record.SetString(0, "Installation failed. Please check the log file at:\nC:\\ProgramData\\Releem\\logs\\releem-install.log\n\nIf you need assistance, please contact hello@releem.com");
                            session.Message(InstallMessage.Error, record);
                        }

                        return ActionResult.Failure;
                    }
                }

                session.Log("RunInstaller completed successfully");
                return ActionResult.Success;
            }
            catch (Exception ex)
            {
                session.Log($"ERROR in RunInstaller: {ex.Message}");
                session.Log($"Stack trace: {ex.StackTrace}");

                using (Record record = new Record(0))
                {
                    record.SetString(0, $"Installation error: {ex.Message}\n\nPlease contact hello@releem.com for assistance.");
                    session.Message(InstallMessage.Error, record);
                }

                return ActionResult.Failure;
            }
        }

        /// <summary>
        /// Run install.ps1 -Uninstall to clean up
        /// </summary>
        [CustomAction]
        public static ActionResult RunUninstaller(Session session)
        {
            session.Log("Begin RunUninstaller");

            try
            {
                // Parse custom action data
                var data = session.CustomActionData;
                string installDir = data["INSTALLDIR"];

                session.Log($"Install directory: {installDir}");

                if (string.IsNullOrEmpty(installDir))
                {
                    session.Log("WARNING: Install directory is not set, skipping uninstall script");
                    return ActionResult.Success; // Don't fail uninstall if directory not found
                }

                // Build path to install.ps1
                string installScript = Path.Combine(installDir, "install.ps1");

                if (!File.Exists(installScript))
                {
                    session.Log($"WARNING: install.ps1 not found at {installScript}, skipping uninstall script");
                    return ActionResult.Success; // Don't fail uninstall if script not found
                }

                session.Log($"Found install.ps1 at: {installScript}");

                // Build PowerShell arguments for uninstall
                string arguments = $"-ExecutionPolicy Bypass -NoProfile -WindowStyle Hidden -File \"{installScript}\" -Uninstall -Silent";

                session.Log($"Executing PowerShell with arguments: {arguments}");

                // Start PowerShell process
                var processInfo = new ProcessStartInfo
                {
                    FileName = "powershell.exe",
                    Arguments = arguments,
                    UseShellExecute = false,
                    RedirectStandardOutput = true,
                    RedirectStandardError = true,
                    CreateNoWindow = true
                };

                using (var process = Process.Start(processInfo))
                {
                    // Read output and errors
                    string output = process.StandardOutput.ReadToEnd();
                    string errors = process.StandardError.ReadToEnd();

                    process.WaitForExit();

                    // Log output
                    if (!string.IsNullOrEmpty(output))
                    {
                        session.Log($"PowerShell Output: {output}");
                    }

                    if (!string.IsNullOrEmpty(errors))
                    {
                        session.Log($"PowerShell Errors: {errors}");
                    }

                    session.Log($"PowerShell exit code: {process.ExitCode}");

                    // Don't fail uninstall even if script fails
                    if (process.ExitCode != 0)
                    {
                        session.Log($"WARNING: install.ps1 -Uninstall returned exit code {process.ExitCode}, but continuing with uninstall");
                    }
                }

                session.Log("RunUninstaller completed");
                return ActionResult.Success;
            }
            catch (Exception ex)
            {
                session.Log($"WARNING in RunUninstaller: {ex.Message}");
                session.Log($"Stack trace: {ex.StackTrace}");

                // Don't fail uninstall on errors
                return ActionResult.Success;
            }
        }
    }
}
