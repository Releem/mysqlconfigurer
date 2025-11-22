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
        private const string DownloadUrl = "https://releem.s3.us-east-1.amazonaws.com/v2/releem-agent.exe";
        private const string ExecutableName = "releem-agent.exe";
        private const string ConfigFileName = "releem.conf";

        [CustomAction]
        public static ActionResult DownloadExecutable(Session session)
        {
            session.Log("Starting DownloadExecutable custom action");

            try
            {
                string installDir = session["INSTALLDIR"];
                if (string.IsNullOrEmpty(installDir))
                {
                    installDir = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles), "ReleemAgent");
                }

                session.Log($"Install directory: {installDir}");

                // Ensure directory exists
                if (!Directory.Exists(installDir))
                {
                    Directory.CreateDirectory(installDir);
                    session.Log($"Created directory: {installDir}");
                }

                string targetPath = Path.Combine(installDir, ExecutableName);
                session.Log($"Downloading from: {DownloadUrl}");
                session.Log($"Downloading to: {targetPath}");

                // Download the file
                ServicePointManager.SecurityProtocol = SecurityProtocolType.Tls12;
                using (WebClient client = new WebClient())
                {
                    client.DownloadFile(DownloadUrl, targetPath);
                }

                if (File.Exists(targetPath))
                {
                    FileInfo fileInfo = new FileInfo(targetPath);
                    session.Log($"Download successful. File size: {fileInfo.Length} bytes");
                    return ActionResult.Success;
                }
                else
                {
                    session.Log("ERROR: Downloaded file not found");
                    return ActionResult.Failure;
                }
            }
            catch (Exception ex)
            {
                session.Log($"ERROR downloading executable: {ex.Message}");
                session.Log($"Stack trace: {ex.StackTrace}");
                return ActionResult.Failure;
            }
        }

        [CustomAction]
        public static ActionResult GenerateConfigFile(Session session)
        {
            session.Log("Starting GenerateConfigFile custom action");

            try
            {
                // Parse custom action data for deferred execution
                string customActionData = session["CustomActionData"];
                var properties = ParseCustomActionData(customActionData);

                string configDir = GetProperty(properties, "CONFIGDIR");
                if (string.IsNullOrEmpty(configDir))
                {
                    configDir = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.CommonApplicationData), "ReleemAgent");
                }

                session.Log($"Config directory: {configDir}");

                // Ensure directory exists
                if (!Directory.Exists(configDir))
                {
                    Directory.CreateDirectory(configDir);
                    session.Log($"Created config directory: {configDir}");
                }

                // Create conf.d subdirectory
                string confDDir = Path.Combine(configDir, "conf.d");
                if (!Directory.Exists(confDDir))
                {
                    Directory.CreateDirectory(confDDir);
                    session.Log($"Created conf.d directory: {confDDir}");
                }

                string apiKey = GetProperty(properties, "RELEEM_APIKEY");
                string mysqlHost = GetProperty(properties, "MYSQL_HOST", "127.0.0.1");
                string mysqlPort = GetProperty(properties, "MYSQL_PORT", "3306");
                string mysqlUser = GetProperty(properties, "MYSQL_USER", "releem");
                string mysqlPassword = GetProperty(properties, "MYSQL_PASSWORD");
                string queryOptimization = GetProperty(properties, "QUERY_OPTIMIZATION", "true");

                session.Log($"API Key length: {apiKey?.Length ?? 0}");
                session.Log($"MySQL Host: {mysqlHost}");
                session.Log($"MySQL Port: {mysqlPort}");
                session.Log($"MySQL User: {mysqlUser}");
                session.Log($"Query Optimization: {queryOptimization}");

                // Build configuration content
                StringBuilder configContent = new StringBuilder();
                configContent.AppendLine($"apikey=\"{apiKey}\"");
                configContent.AppendLine($"mysql_host=\"{mysqlHost}\"");
                configContent.AppendLine($"mysql_port=\"{mysqlPort}\"");
                configContent.AppendLine($"mysql_user=\"{mysqlUser}\"");
                configContent.AppendLine($"mysql_password=\"{mysqlPassword}\"");
                configContent.AppendLine($"query_optimization={queryOptimization}");

                string configPath = Path.Combine(configDir, ConfigFileName);
                File.WriteAllText(configPath, configContent.ToString(), Encoding.UTF8);

                session.Log($"Configuration file created: {configPath}");
                return ActionResult.Success;
            }
            catch (Exception ex)
            {
                session.Log($"ERROR generating config file: {ex.Message}");
                session.Log($"Stack trace: {ex.StackTrace}");
                return ActionResult.Failure;
            }
        }

        [CustomAction]
        public static ActionResult InstallService(Session session)
        {
            session.Log("Starting InstallService custom action");

            try
            {
                string customActionData = session["CustomActionData"];
                var properties = ParseCustomActionData(customActionData);

                string installDir = GetProperty(properties, "INSTALLDIR");
                if (string.IsNullOrEmpty(installDir))
                {
                    installDir = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles), "ReleemAgent");
                }

                string exePath = Path.Combine(installDir, ExecutableName);
                session.Log($"Executable path: {exePath}");

                if (!File.Exists(exePath))
                {
                    session.Log($"ERROR: Executable not found at {exePath}");
                    return ActionResult.Failure;
                }

                var result = RunCommand(exePath, "install", session);
                return result ? ActionResult.Success : ActionResult.Failure;
            }
            catch (Exception ex)
            {
                session.Log($"ERROR installing service: {ex.Message}");
                session.Log($"Stack trace: {ex.StackTrace}");
                return ActionResult.Failure;
            }
        }

        [CustomAction]
        public static ActionResult StartService(Session session)
        {
            session.Log("Starting StartService custom action");

            try
            {
                string customActionData = session["CustomActionData"];
                var properties = ParseCustomActionData(customActionData);

                string installDir = GetProperty(properties, "INSTALLDIR");
                if (string.IsNullOrEmpty(installDir))
                {
                    installDir = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles), "ReleemAgent");
                }

                string exePath = Path.Combine(installDir, ExecutableName);
                session.Log($"Executable path: {exePath}");

                var result = RunCommand(exePath, "start", session);
                return result ? ActionResult.Success : ActionResult.Failure;
            }
            catch (Exception ex)
            {
                session.Log($"ERROR starting service: {ex.Message}");
                return ActionResult.Failure;
            }
        }

        [CustomAction]
        public static ActionResult StopService(Session session)
        {
            session.Log("Starting StopService custom action");

            try
            {
                string customActionData = session["CustomActionData"];
                var properties = ParseCustomActionData(customActionData);

                string installDir = GetProperty(properties, "INSTALLDIR");
                if (string.IsNullOrEmpty(installDir))
                {
                    installDir = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles), "ReleemAgent");
                }

                string exePath = Path.Combine(installDir, ExecutableName);
                session.Log($"Executable path: {exePath}");

                // Don't fail if service isn't running
                RunCommand(exePath, "stop", session);
                return ActionResult.Success;
            }
            catch (Exception ex)
            {
                session.Log($"Warning stopping service: {ex.Message}");
                return ActionResult.Success; // Don't fail uninstall if stop fails
            }
        }

        [CustomAction]
        public static ActionResult RemoveService(Session session)
        {
            session.Log("Starting RemoveService custom action");

            try
            {
                string customActionData = session["CustomActionData"];
                var properties = ParseCustomActionData(customActionData);

                string installDir = GetProperty(properties, "INSTALLDIR");
                if (string.IsNullOrEmpty(installDir))
                {
                    installDir = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles), "ReleemAgent");
                }

                string exePath = Path.Combine(installDir, ExecutableName);
                session.Log($"Executable path: {exePath}");

                // Don't fail if service doesn't exist
                RunCommand(exePath, "remove", session);
                return ActionResult.Success;
            }
            catch (Exception ex)
            {
                session.Log($"Warning removing service: {ex.Message}");
                return ActionResult.Success; // Don't fail uninstall if remove fails
            }
        }

        private static bool RunCommand(string exePath, string arguments, Session session)
        {
            session.Log($"Running: \"{exePath}\" {arguments}");

            ProcessStartInfo psi = new ProcessStartInfo
            {
                FileName = exePath,
                Arguments = arguments,
                UseShellExecute = false,
                RedirectStandardOutput = true,
                RedirectStandardError = true,
                CreateNoWindow = true
            };

            using (Process process = Process.Start(psi))
            {
                string output = process.StandardOutput.ReadToEnd();
                string error = process.StandardError.ReadToEnd();
                process.WaitForExit();

                session.Log($"Exit code: {process.ExitCode}");
                if (!string.IsNullOrEmpty(output))
                    session.Log($"Output: {output}");
                if (!string.IsNullOrEmpty(error))
                    session.Log($"Error: {error}");

                return process.ExitCode == 0;
            }
        }

        private static System.Collections.Generic.Dictionary<string, string> ParseCustomActionData(string data)
        {
            var result = new System.Collections.Generic.Dictionary<string, string>(StringComparer.OrdinalIgnoreCase);

            if (string.IsNullOrEmpty(data))
                return result;

            foreach (var pair in data.Split(';'))
            {
                var index = pair.IndexOf('=');
                if (index > 0)
                {
                    var key = pair.Substring(0, index);
                    var value = pair.Substring(index + 1);
                    result[key] = value;
                }
            }

            return result;
        }

        private static string GetProperty(System.Collections.Generic.Dictionary<string, string> properties, string key, string defaultValue = "")
        {
            if (properties.TryGetValue(key, out string value) && !string.IsNullOrEmpty(value))
                return value;
            return defaultValue;
        }
    }
}
