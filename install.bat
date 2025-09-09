@echo off
REM install.bat - Version 1.22.0
REM (C) Releem, Inc 2022
REM All rights reserved

REM Releem installation script: install and set up the Releem Agent on Windows
REM using PowerShell and Windows commands.

setlocal enabledelayedexpansion

set INSTALL_SCRIPT_VERSION=1.22.0
set LOGFILE=%TEMP%\releem-install.log

set WORKDIR=C:\Program Files\ReleemAgent
set CONF=%WORKDIR%\releem.conf
set MYSQL_CONF_DIR=%WORKDIR%\conf
set RELEEM_COMMAND="%WORKDIR%\mysqlconfigurer.bat"

REM Initialize log file
echo. > "%LOGFILE%"
echo Starting Releem installation at %DATE% %TIME% >> "%LOGFILE%"

REM Function to log messages
goto :main

:log_message
echo %~1 >> "%LOGFILE%"
echo %~1
goto :eof

:on_error
call :log_message "ERROR: %ERROR_MESSAGE%"
call :log_message "It looks like you encountered an issue while installing Releem."
call :log_message "If you are still experiencing problems, please send an email to hello@releem.com"
call :log_message "with the contents of %LOGFILE%. We will do our best to resolve the issue."

REM Send log to API
if defined RELEEM_REGION (
    if /i "%RELEEM_REGION%"=="EU" (
        set API_DOMAIN=api.eu.releem.com
    ) else (
        set API_DOMAIN=api.releem.com
    )
) else (
    set API_DOMAIN=api.releem.com
)

REM Use PowerShell to send the log file
powershell -Command "try { $headers = @{'x-releem-api-key'='%apikey%'; 'Content-Type'='application/json'}; Invoke-RestMethod -Uri 'https://%API_DOMAIN%/v2/events/saving_log' -Method Post -InFile '%LOGFILE%' -Headers $headers } catch { Write-Host 'Failed to send log to API' }"
goto :eof

:releem_update
call :log_message "* Downloading latest version of Releem Agent."

REM Check if releem-agent.exe exists and stop it
if exist "%WORKDIR%\releem-agent.exe" (
    call :log_message "Stopping existing Releem Agent..."
    "%WORKDIR%\releem-agent.exe" stop 2>>"%LOGFILE%"
)

REM Download new version
powershell -Command "Invoke-WebRequest -Uri 'https://releem.s3.amazonaws.com/v2/releem-agent-windows.exe' -OutFile '%WORKDIR%\releem-agent.new'" 2>>"%LOGFILE%"
if !errorlevel! neq 0 (
    set ERROR_MESSAGE=Failed to download Releem Agent
    goto :on_error
)

powershell -Command "Invoke-WebRequest -Uri 'https://releem.s3.amazonaws.com/v2/mysqlconfigurer.bat' -OutFile '%WORKDIR%\mysqlconfigurer.bat.new'" 2>>"%LOGFILE%"
if !errorlevel! neq 0 (
    set ERROR_MESSAGE=Failed to download MySQL configurer script
    goto :on_error
)

REM Replace files
move "%WORKDIR%\releem-agent.new" "%WORKDIR%\releem-agent.exe" 2>>"%LOGFILE%"
move "%WORKDIR%\mysqlconfigurer.bat.new" "%WORKDIR%\mysqlconfigurer.bat" 2>>"%LOGFILE%"

REM Start the service
"%WORKDIR%\releem-agent.exe" start 2>>"%LOGFILE%"
"%WORKDIR%\releem-agent.exe" -f 2>>"%LOGFILE%"

call :log_message ""
call :log_message "Releem Agent updated successfully."
call :log_message ""
call :log_message "To check MySQL Performance Score please visit https://app.releem.com/dashboard?menu=metrics"
call :log_message ""
exit /b 0

:releem_uninstall
call :log_message "* Uninstalling Releem Agent"

REM Send uninstall event
if exist "%WORKDIR%\releem-agent.exe" (
    "%WORKDIR%\releem-agent.exe" --event=agent_uninstall > nul 2>&1
)

call :log_message "* Removing scheduled task"
schtasks /delete /tn "Releem Agent Config Update" /f > nul 2>&1

call :log_message "* Stopping Releem Agent service"
if exist "%WORKDIR%\releem-agent.exe" (
    "%WORKDIR%\releem-agent.exe" stop 2>>"%LOGFILE%"
    if !errorlevel! equ 0 (
        call :log_message "Releem Agent stopped successfully."
    ) else (
        call :log_message "Releem Agent failed to stop."
    )
)

call :log_message "* Uninstalling Releem Agent service"
if exist "%WORKDIR%\releem-agent.exe" (
    "%WORKDIR%\releem-agent.exe" remove 2>>"%LOGFILE%"
    if !errorlevel! equ 0 (
        call :log_message "Releem Agent uninstalled successfully."
    ) else (
        call :log_message "Releem Agent failed to uninstall."
    )
)

call :log_message "* Removing Releem files"
rmdir /s /q "%WORKDIR%" 2>>"%LOGFILE%"
exit /b 0

:enable_query_optimization
call :log_message "* Enabling Query Optimization"

REM Grant SELECT privileges to releem user
for /f "tokens=*" %%i in ('mysql %root_connection_string% --user=root --password=%RELEEM_MYSQL_ROOT_PASSWORD% -NBe "select Concat(\"GRANT SELECT on *.* to `\",User,\"`@`\", Host,\"`;\") from mysql.user where User=\"releem\""') do (
    call :log_message "Executing: %%i"
    mysql %root_connection_string% --user=root --password=%RELEEM_MYSQL_ROOT_PASSWORD% -Be "%%i" 2>>"%LOGFILE%"
)

REM Add query optimization to config if not present
findstr /c:"query_optimization" "%CONF%" > nul
if !errorlevel! neq 0 (
    echo query_optimization=true >> "%CONF%"
)

REM Restart Releem Agent
call :log_message "* Restarting Releem Agent"
"%WORKDIR%\releem-agent.exe" stop 2>>"%LOGFILE%"
"%WORKDIR%\releem-agent.exe" start 2>>"%LOGFILE%"
if !errorlevel! equ 0 (
    call :log_message "Restarting Releem Agent - successful"
) else (
    call :log_message "Restarting Releem Agent - failed"
)

timeout /t 3 > nul

REM Check if process is running
tasklist /fi "imagename eq releem-agent.exe" | find "releem-agent.exe" > nul
if !errorlevel! neq 0 (
    set ERROR_MESSAGE=The releem-agent process was not found! Check the system log for an error.
    goto :on_error
)

REM Enable performance schema
call "%WORKDIR%\mysqlconfigurer.bat" -p
exit /b 0

:main
REM Check for command line arguments
if "%1"=="uninstall" goto :releem_uninstall
if "%1"=="enable_query_optimization" goto :enable_query_optimization
if "%1"=="-u" goto :releem_update

REM Check for API key
if defined RELEEM_API_KEY (
    set apikey=%RELEEM_API_KEY%
) else (
    if exist "%CONF%" (
        for /f "tokens=2 delims==" %%a in ('findstr "apikey" "%CONF%" 2^>nul') do (
            set apikey=%%a
            set apikey=!apikey:"=!
        )
    )
)

if not defined apikey (
    set ERROR_MESSAGE=Releem API key is not available in RELEEM_API_KEY environment variable. Please sign up at https://releem.com
    goto :on_error
)

call :log_message "* Checking for MySQL installation"

REM Find MySQL executables
set mysqladmincmd=
set mysqlcmd=

REM Check common MySQL installation paths
for %%p in (
    "C:\Program Files\MySQL\MySQL Server 8.0\bin\mysqladmin.exe"
    "C:\Program Files\MySQL\MySQL Server 5.7\bin\mysqladmin.exe"
    "C:\Program Files (x86)\MySQL\MySQL Server 8.0\bin\mysqladmin.exe"
    "C:\Program Files (x86)\MySQL\MySQL Server 5.7\bin\mysqladmin.exe"
    "C:\mysql\bin\mysqladmin.exe"
    "C:\xampp\mysql\bin\mysqladmin.exe"
) do (
    if exist %%p (
        set mysqladmincmd=%%p
        goto :found_mysqladmin
    )
)

REM Try to find in PATH
where mysqladmin.exe > nul 2>&1
if !errorlevel! equ 0 (
    for /f "tokens=*" %%i in ('where mysqladmin.exe') do set mysqladmincmd=%%i
)

:found_mysqladmin
if not defined mysqladmincmd (
    set ERROR_MESSAGE=Couldn't find mysqladmin.exe. Please ensure MySQL is installed and accessible.
    goto :on_error
)

REM Find mysql.exe
for %%p in (
    "C:\Program Files\MySQL\MySQL Server 8.0\bin\mysql.exe"
    "C:\Program Files\MySQL\MySQL Server 5.7\bin\mysql.exe"
    "C:\Program Files (x86)\MySQL\MySQL Server 8.0\bin\mysql.exe"
    "C:\Program Files (x86)\MySQL\MySQL Server 5.7\bin\mysql.exe"
    "C:\mysql\bin\mysql.exe"
    "C:\xampp\mysql\bin\mysql.exe"
) do (
    if exist %%p (
        set mysqlcmd=%%p
        goto :found_mysql
    )
)

REM Try to find in PATH
where mysql.exe > nul 2>&1
if !errorlevel! equ 0 (
    for /f "tokens=*" %%i in ('where mysql.exe') do set mysqlcmd=%%i
)

:found_mysql
if not defined mysqlcmd (
    set ERROR_MESSAGE=Couldn't find mysql.exe. Please ensure MySQL is installed and accessible.
    goto :on_error
)

call :log_message "* Found MySQL at: !mysqlcmd!"
call :log_message "* Found mysqladmin at: !mysqladmincmd!"

REM Build connection string
set connection_string=
set root_connection_string=

if defined RELEEM_MYSQL_HOST (
    REM Check if it's a socket (not applicable on Windows, treat as host)
    set mysql_user_host=%RELEEM_MYSQL_HOST%
    if "%RELEEM_MYSQL_HOST%"=="127.0.0.1" set mysql_user_host=127.0.0.1
    if "%RELEEM_MYSQL_HOST%"=="localhost" set mysql_user_host=localhost
    if not "%RELEEM_MYSQL_HOST%"=="127.0.0.1" if not "%RELEEM_MYSQL_HOST%"=="localhost" set mysql_user_host=%%
    
    set connection_string=!connection_string! --host=%RELEEM_MYSQL_HOST%
    set root_connection_string=!root_connection_string! --host=%RELEEM_MYSQL_HOST%
    
    if defined RELEEM_MYSQL_PORT (
        set connection_string=!connection_string! --port=%RELEEM_MYSQL_PORT%
        set root_connection_string=!root_connection_string! --port=%RELEEM_MYSQL_PORT%
    ) else (
        set connection_string=!connection_string! --port=3306
        set root_connection_string=!root_connection_string! --port=3306
    )
) else (
    set mysql_user_host=127.0.0.1
    set connection_string=!connection_string! --host=127.0.0.1
    set root_connection_string=!root_connection_string! --host=127.0.0.1
    
    if defined RELEEM_MYSQL_PORT (
        set connection_string=!connection_string! --port=%RELEEM_MYSQL_PORT%
        set root_connection_string=!root_connection_string! --port=%RELEEM_MYSQL_PORT%
    ) else (
        set connection_string=!connection_string! --port=3306
        set root_connection_string=!root_connection_string! --port=3306
    )
)

call :log_message "* Creating work directory"
if not exist "%WORKDIR%" mkdir "%WORKDIR%" 2>>"%LOGFILE%"
if not exist "%WORKDIR%\conf" mkdir "%WORKDIR%\conf" 2>>"%LOGFILE%"

call :log_message "* Downloading Releem Agent for Windows"
powershell -Command "Invoke-WebRequest -Uri 'https://releem.s3.amazonaws.com/v2/mysqlconfigurer.bat' -OutFile '%WORKDIR%\mysqlconfigurer.bat'" 2>>"%LOGFILE%"
if !errorlevel! neq 0 (
    set ERROR_MESSAGE=Failed to download MySQL configurer script
    goto :on_error
)

powershell -Command "Invoke-WebRequest -Uri 'https://releem.s3.amazonaws.com/v2/releem-agent-windows.exe' -OutFile '%WORKDIR%\releem-agent.exe'" 2>>"%LOGFILE%"
if !errorlevel! neq 0 (
    set ERROR_MESSAGE=Failed to download Releem Agent
    goto :on_error
)

call :log_message "* Configuring MySQL user for metrics collection"
set FLAG_SUCCESS=0

if defined RELEEM_MYSQL_PASSWORD if defined RELEEM_MYSQL_LOGIN (
    call :log_message "* Using MySQL login and password from environment variables"
    set FLAG_SUCCESS=1
) else (
    call :log_message "* Using MySQL root user"
    
    REM Test MySQL connection
    "!mysqladmincmd!" !root_connection_string! --user=root --password=%RELEEM_MYSQL_ROOT_PASSWORD% ping > nul 2>&1
    if !errorlevel! equ 0 (
        call :log_message "MySQL connection successful."
        
        set RELEEM_MYSQL_LOGIN=releem
        
        REM Generate random password (simplified version for Windows)
        set RELEEM_MYSQL_PASSWORD=Releem_%RANDOM%%RANDOM%_Pass
        
        REM Create MySQL user
        "!mysqlcmd!" !root_connection_string! --user=root --password=%RELEEM_MYSQL_ROOT_PASSWORD% -Be "DROP USER IF EXISTS '%RELEEM_MYSQL_LOGIN%'@'!mysql_user_host!';" 2>>"%LOGFILE%"
        "!mysqlcmd!" !root_connection_string! --user=root --password=%RELEEM_MYSQL_ROOT_PASSWORD% -Be "CREATE USER '%RELEEM_MYSQL_LOGIN%'@'!mysql_user_host!' identified by '%RELEEM_MYSQL_PASSWORD%';" 2>>"%LOGFILE%"
        if !errorlevel! neq 0 (
            set ERROR_MESSAGE=Failed to create MySQL user
            goto :on_error
        )
        
        REM Grant privileges
        "!mysqlcmd!" !root_connection_string! --user=root --password=%RELEEM_MYSQL_ROOT_PASSWORD% -Be "GRANT PROCESS ON *.* TO '%RELEEM_MYSQL_LOGIN%'@'!mysql_user_host!';" 2>>"%LOGFILE%"
        "!mysqlcmd!" !root_connection_string! --user=root --password=%RELEEM_MYSQL_ROOT_PASSWORD% -Be "GRANT REPLICATION CLIENT ON *.* TO '%RELEEM_MYSQL_LOGIN%'@'!mysql_user_host!';" 2>>"%LOGFILE%"
        "!mysqlcmd!" !root_connection_string! --user=root --password=%RELEEM_MYSQL_ROOT_PASSWORD% -Be "GRANT SHOW VIEW ON *.* TO '%RELEEM_MYSQL_LOGIN%'@'!mysql_user_host!';" 2>>"%LOGFILE%"
        "!mysqlcmd!" !root_connection_string! --user=root --password=%RELEEM_MYSQL_ROOT_PASSWORD% -Be "GRANT SELECT ON mysql.* TO '%RELEEM_MYSQL_LOGIN%'@'!mysql_user_host!';" 2>>"%LOGFILE%"
        
        REM Grant performance schema privileges (ignore errors for older versions)
        "!mysqlcmd!" !root_connection_string! --user=root --password=%RELEEM_MYSQL_ROOT_PASSWORD% -Be "GRANT SELECT ON performance_schema.events_statements_summary_by_digest TO '%RELEEM_MYSQL_LOGIN%'@'!mysql_user_host!';" 2>>"%LOGFILE%"
        "!mysqlcmd!" !root_connection_string! --user=root --password=%RELEEM_MYSQL_ROOT_PASSWORD% -Be "GRANT SELECT ON performance_schema.table_io_waits_summary_by_index_usage TO '%RELEEM_MYSQL_LOGIN%'@'!mysql_user_host!';" 2>>"%LOGFILE%"
        "!mysqlcmd!" !root_connection_string! --user=root --password=%RELEEM_MYSQL_ROOT_PASSWORD% -Be "GRANT SELECT ON performance_schema.file_summary_by_instance TO '%RELEEM_MYSQL_LOGIN%'@'!mysql_user_host!';" 2>>"%LOGFILE%"
        
        REM Grant SUPER or SYSTEM_VARIABLES_ADMIN privilege
        "!mysqlcmd!" !root_connection_string! --user=root --password=%RELEEM_MYSQL_ROOT_PASSWORD% -Be "GRANT SYSTEM_VARIABLES_ADMIN ON *.* TO '%RELEEM_MYSQL_LOGIN%'@'!mysql_user_host!';" 2>>"%LOGFILE%"
        if !errorlevel! neq 0 (
            "!mysqlcmd!" !root_connection_string! --user=root --password=%RELEEM_MYSQL_ROOT_PASSWORD% -Be "GRANT SUPER ON *.* TO '%RELEEM_MYSQL_LOGIN%'@'!mysql_user_host!';" 2>>"%LOGFILE%"
        )
        
        if defined RELEEM_QUERY_OPTIMIZATION (
            "!mysqlcmd!" !root_connection_string! --user=root --password=%RELEEM_MYSQL_ROOT_PASSWORD% -Be "GRANT SELECT ON *.* TO '%RELEEM_MYSQL_LOGIN%'@'!mysql_user_host!';" 2>>"%LOGFILE%"
        )
        
        call :log_message "Created new user `%RELEEM_MYSQL_LOGIN%`"
        set FLAG_SUCCESS=1
    ) else (
        set ERROR_MESSAGE=MySQL connection failed with user root. Check that the password is correct.
        goto :on_error
    )
)

if "%FLAG_SUCCESS%"=="1" (
    REM Test connection with created user
    "!mysqladmincmd!" !connection_string! --user=%RELEEM_MYSQL_LOGIN% --password=%RELEEM_MYSQL_PASSWORD% ping > nul 2>&1
    if !errorlevel! equ 0 (
        call :log_message "MySQL connection with user `%RELEEM_MYSQL_LOGIN%` - successful."
        set MYSQL_LOGIN=%RELEEM_MYSQL_LOGIN%
        set MYSQL_PASSWORD=%RELEEM_MYSQL_PASSWORD%
    ) else (
        set ERROR_MESSAGE=MySQL connection failed with user `%RELEEM_MYSQL_LOGIN%`. Check that the user and password are correct.
        goto :on_error
    )
)

call :log_message "* Configuring MySQL memory limit"
if defined RELEEM_MYSQL_MEMORY_LIMIT (
    if %RELEEM_MYSQL_MEMORY_LIMIT% gtr 0 (
        set MYSQL_LIMIT=%RELEEM_MYSQL_MEMORY_LIMIT%
    )
) else (
    call :log_message "In case you are using MySQL in Docker or it isn't a dedicated server for MySQL."
    set /p REPLY="Should we limit memory for MySQL database? (Y/N): "
    if /i "!REPLY!"=="Y" (
        set /p MYSQL_LIMIT="Please set MySQL Memory Limit (megabytes): "
    )
)

call :log_message "* Saving variables to Releem Agent configuration"

REM Create configuration file
echo apikey="%apikey%" > "%CONF%"

if exist "%WORKDIR%\conf" (
    echo releem_cnf_dir="%WORKDIR%\conf" >> "%CONF%"
)

if defined MYSQL_LOGIN if defined MYSQL_PASSWORD (
    echo mysql_user="%MYSQL_LOGIN%" >> "%CONF%"
    echo mysql_password="%MYSQL_PASSWORD%" >> "%CONF%"
)

if defined RELEEM_MYSQL_HOST (
    echo mysql_host="%RELEEM_MYSQL_HOST%" >> "%CONF%"
)

if defined RELEEM_MYSQL_PORT (
    echo mysql_port="%RELEEM_MYSQL_PORT%" >> "%CONF%"
)

if defined MYSQL_LIMIT (
    echo memory_limit="%MYSQL_LIMIT%" >> "%CONF%"
)

REM Windows service restart command (will be handled by the agent)
echo mysql_restart_service="net stop mysql && net start mysql" >> "%CONF%"

if exist "%MYSQL_CONF_DIR%" (
    echo mysql_cnf_dir="%MYSQL_CONF_DIR%" >> "%CONF%"
)

if defined RELEEM_HOSTNAME (
    echo hostname="%RELEEM_HOSTNAME%" >> "%CONF%"
) else (
    for /f "tokens=*" %%i in ('hostname') do (
        echo hostname="%%i" >> "%CONF%"
    )
)

if defined RELEEM_ENV (
    echo env="%RELEEM_ENV%" >> "%CONF%"
)

if defined RELEEM_DEBUG (
    echo debug=%RELEEM_DEBUG% >> "%CONF%"
)

if defined RELEEM_MYSQL_SSL_MODE (
    echo mysql_ssl_mode=%RELEEM_MYSQL_SSL_MODE% >> "%CONF%"
)

if defined RELEEM_QUERY_OPTIMIZATION (
    echo query_optimization=%RELEEM_QUERY_OPTIMIZATION% >> "%CONF%"
)

if defined RELEEM_DATABASES_QUERY_OPTIMIZATION (
    echo databases_query_optimization="%RELEEM_DATABASES_QUERY_OPTIMIZATION%" >> "%CONF%"
)

if defined RELEEM_REGION (
    echo releem_region="%RELEEM_REGION%" >> "%CONF%"
)

echo interval_seconds=60 >> "%CONF%"
echo interval_read_config_seconds=3600 >> "%CONF%"

call :log_message "* Configuring scheduled task"
set RELEEM_TASK_NAME=Releem Agent Config Update
set RELEEM_TASK_COMMAND="%WORKDIR%\mysqlconfigurer.bat" -u

if not defined RELEEM_CRON_ENABLE (
    call :log_message "To get recommendations, we need to create a scheduled task:"
    call :log_message "Task: %RELEEM_TASK_NAME%"
    call :log_message "Command: %RELEEM_TASK_COMMAND%"
    call :log_message "Schedule: Daily at midnight"
    set /p REPLY="Can we create this scheduled task automatically? (Y/N): "
    if /i "!REPLY!"=="Y" (
        goto :create_scheduled_task
    )
) else (
    if %RELEEM_CRON_ENABLE% gtr 0 (
        goto :create_scheduled_task
    )
)
goto :skip_scheduled_task

:create_scheduled_task
schtasks /create /tn "%RELEEM_TASK_NAME%" /tr "%RELEEM_TASK_COMMAND%" /sc daily /st 00:00 /f > nul 2>&1
if !errorlevel! equ 0 (
    call :log_message "Scheduled task configuration complete. Automatic updates are enabled."
) else (
    call :log_message "Scheduled task configuration failed. Automatic updates are disabled."
)

:skip_scheduled_task

if not defined RELEEM_AGENT_DISABLE (
    call :log_message "* Executing Releem Agent for the first time"
    call :log_message "This may take up to 15 minutes on servers with many databases."
    "%WORKDIR%\releem-agent.exe" -f 2>>"%LOGFILE%"
    timeout /t 3 > nul
    "%WORKDIR%\releem-agent.exe" 2>>"%LOGFILE%"
)

call :log_message "* Installing and starting Releem Agent service to collect metrics"
"%WORKDIR%\releem-agent.exe" remove 2>>"%LOGFILE%"
"%WORKDIR%\releem-agent.exe" install 2>>"%LOGFILE%"
if !errorlevel! equ 0 (
    call :log_message "The Releem Agent installation successful."
) else (
    call :log_message "The Releem Agent installation failed."
)

"%WORKDIR%\releem-agent.exe" stop 2>>"%LOGFILE%"
"%WORKDIR%\releem-agent.exe" start 2>>"%LOGFILE%"
if !errorlevel! equ 0 (
    call :log_message "The Releem Agent restart successful."
) else (
    call :log_message "The Releem Agent restart failed."
)

timeout /t 3 > nul

REM Check if process is running
tasklist /fi "imagename eq releem-agent.exe" | find "releem-agent.exe" > nul
if !errorlevel! neq 0 (
    set ERROR_MESSAGE=The releem-agent process was not found! Check the system log for an error.
    goto :on_error
)

REM Enable performance schema
call "%WORKDIR%\mysqlconfigurer.bat" -p

call :log_message ""
call :log_message "* Releem Agent has been successfully installed."
call :log_message ""
call :log_message "* To view Releem recommendations and MySQL metrics, visit https://app.releem.com/dashboard"
call :log_message ""

exit /b 0
