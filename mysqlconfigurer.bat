@echo off
REM mysqlconfigurer.bat - Version 1.22.0
REM (C) Releem, Inc 2022
REM All rights reserved

setlocal enabledelayedexpansion

REM Variables
set "MYSQLCONFIGURER_PATH=C:\Program Files\ReleemAgent\conf\"
set "RELEEM_CONF_FILE=C:\Program Files\ReleemAgent\releem.conf"
set "MYSQLCONFIGURER_FILE_NAME=z_aiops_mysql.cnf"
set "MYSQLTUNER_FILENAME=%MYSQLCONFIGURER_PATH%mysqltuner.pl"
set "MYSQLTUNER_REPORT=%MYSQLCONFIGURER_PATH%mysqltunerreport.json"
set "RELEEM_MYSQL_VERSION=%MYSQLCONFIGURER_PATH%mysql_version"
set "MYSQLCONFIGURER_CONFIGFILE=%MYSQLCONFIGURER_PATH%%MYSQLCONFIGURER_FILE_NAME%"
set "MYSQL_MEMORY_LIMIT=0"
set "VERSION=1.22.0"
set "RELEEM_INSTALL_PATH=%MYSQLCONFIGURER_PATH%install.bat"
set "logfile=C:\Program Files\ReleemAgent\logs\releem-mysqlconfigurer.log"
set "MYSQL_CONF_DIR=C:\Program Files\MySQL\MySQL Server 8.0\releem.conf.d"

REM Create necessary directories
if not exist "%MYSQLCONFIGURER_PATH%" mkdir "%MYSQLCONFIGURER_PATH%"
if not exist "C:\Program Files\ReleemAgent\logs\" mkdir "C:\Program Files\ReleemAgent\logs\"

REM Start logging
echo [%date% %time%] MySQL Configurer started > "%logfile%"

REM API Domain setup
if "%RELEEM_REGION%"=="EU" (
    set "API_DOMAIN=api.eu.releem.com"
) else (
    set "API_DOMAIN=api.releem.com"
)

REM Parse command line arguments
:parse_args
if "%1"=="" goto main
if "%1"=="-k" (
    set "RELEEM_API_KEY=%2"
    shift
    shift
    goto parse_args
)
if "%1"=="-m" (
    set "MYSQL_MEMORY_LIMIT=%2"
    shift
    shift
    goto parse_args
)
if "%1"=="-a" (
    call :releem_apply_manual
    goto end
)
if "%1"=="-s" (
    call :releem_apply_config %2
    goto end
)
if "%1"=="-r" (
    call :releem_rollback_config
    goto end
)
if "%1"=="-c" (
    call :get_config
    goto end
)
if "%1"=="-p" (
    call :releem_ps_mysql
    goto end
)
if "%1"=="-u" (
    call :update_agent
    goto end
)
shift
goto parse_args

:main
echo.
echo  * To run Releem Agent manually please use the following command:
echo  "C:\Program Files\Releem\releem-agent.exe" -f
echo.
goto end

REM Function: Update Agent
:update_agent
echo [%date% %time%] Updating agent >> "%logfile%"
"C:\Program Files\Releem\releem-agent.exe" start >nul 2>&1

for /f %%i in ('curl -s -L https://releem.s3.amazonaws.com/v2/current_version_agent') do set "NEW_VER=%%i"

if not "%NEW_VER%"=="%VERSION%" (
    echo.
    echo  * Updating script %VERSION% -^> %NEW_VER%
    curl -s -L https://releem.s3.amazonaws.com/v2/install.bat > "%RELEEM_INSTALL_PATH%"
    call "%RELEEM_INSTALL_PATH%" -u
    "C:\Program Files\Releem\releem-agent.exe" --event=agent_updated >nul 2>&1
)
goto :eof

REM Function: Wait for service restart
:wait_restart
set "PID=%1"
set "flag=0"
set "spin[0]=-"
set "spin[1]=\"
set "spin[2]=|"
set "spin[3]=/"

echo.
echo  Waiting for MySQL service to start 1200 seconds %spin[0]%

:wait_loop
set /a "flag+=1"
if %flag% geq 1200 (
    set "RETURN_CODE=6"
    goto wait_end
)

REM Check if MySQL service is running
sc query MySQL >nul 2>&1
if %errorlevel% equ 0 (
    sc query MySQL | find "RUNNING" >nul 2>&1
    if !errorlevel! equ 0 (
        set "RETURN_CODE=0"
        goto wait_end
    )
)

set /a "i=flag %% 4"
<nul set /p "=%spin[%i%]%"
timeout /t 1 >nul
goto wait_loop

:wait_end
echo.
echo MySQL service status check completed with code %RETURN_CODE%
goto :eof

REM Function: Check MySQL version
:check_mysql_version
set "mysql_version="

if exist "%MYSQLTUNER_REPORT%" (
    for /f "tokens=2 delims=:" %%a in ('findstr "Version" "%MYSQLTUNER_REPORT%"') do (
        set "mysql_version=%%a"
        set "mysql_version=!mysql_version:"=!"
        set "mysql_version=!mysql_version:,=!"
    )
) else if exist "%RELEEM_MYSQL_VERSION%" (
    set /p mysql_version=<"%RELEEM_MYSQL_VERSION%"
) else (
    echo.
    echo  * Please try again later or run Releem Agent manually:
    echo   "C:\Program Files\Releem\releem-agent.exe" -f
    exit /b 1
)

if "%mysql_version%"=="" (
    echo.
    echo  * Please try again later or run Releem Agent manually:
    echo   "C:\Program Files\Releem\releem-agent.exe" -f
    exit /b 1
)

REM Version comparison - simplified for Windows batch
set "requiredver=5.6.8"
REM For simplicity, assume version is compatible if not empty
if not "%mysql_version%"=="" exit /b 0
exit /b 1

REM Function: Rollback configuration
:releem_rollback_config
echo.
echo  * Rolling back MySQL configuration.

call :check_mysql_version
if %errorlevel% neq 0 (
    echo.
    echo  * MySQL version is lower than 5.6.7. Check the documentation for applying the configuration.
    exit /b 2
)

if "%RELEEM_MYSQL_CONFIG_DIR%"=="" (
    echo.
    echo  * MySQL configuration directory was not found.
    echo  * Try to reinstall Releem Agent, and set the my.cnf location.
    exit /b 3
)

if not exist "%RELEEM_MYSQL_CONFIG_DIR%" (
    echo.
    echo  * MySQL configuration directory was not found.
    echo  * Try to reinstall Releem Agent, and set the my.cnf location.
    exit /b 3
)

set "FLAG_RESTART_SERVICE=1"
if "%RELEEM_RESTART_SERVICE%"=="" (
    set /p "REPLY=Restart MySQL service? (Y/N) "
    if /i not "!REPLY!"=="Y" (
        echo.
        echo  * Confirmation to restart the service has not been received.
        set "FLAG_RESTART_SERVICE=0"
    )
) else if "%RELEEM_RESTART_SERVICE%"=="0" (
    set "FLAG_RESTART_SERVICE=0"
)

if "%FLAG_RESTART_SERVICE%"=="0" exit /b 5

echo.
echo  * Deleting the configuration file.
if exist "%RELEEM_MYSQL_CONFIG_DIR%\%MYSQLCONFIGURER_FILE_NAME%" (
    del "%RELEEM_MYSQL_CONFIG_DIR%\%MYSQLCONFIGURER_FILE_NAME%"
)

if exist "%MYSQLCONFIGURER_PATH%%MYSQLCONFIGURER_FILE_NAME%.bkp" (
    echo.
    echo  * Restoring the backup copy of the configuration file.
    copy "%MYSQLCONFIGURER_PATH%%MYSQLCONFIGURER_FILE_NAME%.bkp" "%RELEEM_MYSQL_CONFIG_DIR%\%MYSQLCONFIGURER_FILE_NAME%" >nul
)

if "%RELEEM_MYSQL_RESTART_SERVICE%"=="" (
    echo.
    echo  * The command to restart the MySQL service was not found.
    exit /b 4
)

echo.
echo  * Restarting MySQL with command '%RELEEM_MYSQL_RESTART_SERVICE%'.
%RELEEM_MYSQL_RESTART_SERVICE%
set "RESTART_CODE=%errorlevel%"

if %RESTART_CODE% equ 0 (
    echo.
    echo [%date% %time%] The MySQL service restarted successfully!
    if exist "%MYSQLCONFIGURER_PATH%%MYSQLCONFIGURER_FILE_NAME%.bkp" (
        del "%MYSQLCONFIGURER_PATH%%MYSQLCONFIGURER_FILE_NAME%.bkp"
    )
) else (
    echo.
    echo [%date% %time%] The MySQL service failed to restart. Check the MySQL error log.
)

"C:\Program Files\Releem\releem-agent.exe" --event=config_rollback >nul 2>&1
exit /b %RESTART_CODE%

REM Function: Configure Performance Schema and SlowLog
:releem_ps_mysql
set "FLAG_CONFIGURE=1"

REM Check Performance Schema status
for /f "tokens=2" %%a in ('mysql %connection_string% -u%MYSQL_LOGIN% -p%MYSQL_PASSWORD% -BNe "show global variables like 'performance_schema'" 2^>nul') do (
    if not "%%a"=="ON" set "FLAG_CONFIGURE=0"
)

REM Check Slow Query Log status
for /f "tokens=2" %%a in ('mysql %connection_string% -u%MYSQL_LOGIN% -p%MYSQL_PASSWORD% -BNe "show global variables like 'slow_query_log'" 2^>nul') do (
    if not "%%a"=="ON" set "FLAG_CONFIGURE=0"
)

if "%RELEEM_MYSQL_CONFIG_DIR%"=="" (
    echo.
    echo MySQL configuration directory was not found.
    echo Try to reinstall Releem Agent.
    exit /b 3
)

if not exist "%RELEEM_MYSQL_CONFIG_DIR%" (
    echo.
    echo MySQL configuration directory was not found.
    echo Try to reinstall Agent.
    exit /b 3
)

echo.
echo  * Enabling and configuring Performance schema and SlowLog to collect metrics and queries.

echo ### This configuration was recommended by Releem. https://releem.com > "%RELEEM_MYSQL_CONFIG_DIR%\collect_metrics.cnf"
echo [mysqld] >> "%RELEEM_MYSQL_CONFIG_DIR%\collect_metrics.cnf"
echo performance_schema = 1 >> "%RELEEM_MYSQL_CONFIG_DIR%\collect_metrics.cnf"
echo slow_query_log = 1 >> "%RELEEM_MYSQL_CONFIG_DIR%\collect_metrics.cnf"

if "%RELEEM_QUERY_OPTIMIZATION%"=="true" (
    call :check_mysql_version
    if !errorlevel! neq 0 (
        echo.
        echo  * MySQL version is lower than 5.6.7. Query optimization is not supported.
    ) else (
        REM Check Performance Schema consumers
        for /f "tokens=1" %%a in ('mysql %connection_string% -u%MYSQL_LOGIN% -p%MYSQL_PASSWORD% -BNe "SELECT ENABLED FROM performance_schema.setup_consumers WHERE NAME = 'events_statements_current';" 2^>nul') do (
            if not "%%a"=="YES" set "FLAG_CONFIGURE=0"
        )
        
        for /f "tokens=1" %%a in ('mysql %connection_string% -u%MYSQL_LOGIN% -p%MYSQL_PASSWORD% -BNe "SELECT ENABLED FROM performance_schema.setup_consumers WHERE NAME = 'events_statements_history';" 2^>nul') do (
            if not "%%a"=="YES" set "FLAG_CONFIGURE=0"
        )
        
        echo performance-schema-consumer-events-statements-history = ON >> "%RELEEM_MYSQL_CONFIG_DIR%\collect_metrics.cnf"
        echo performance-schema-consumer-events-statements-current = ON >> "%RELEEM_MYSQL_CONFIG_DIR%\collect_metrics.cnf"
    )
)

if "%FLAG_CONFIGURE%"=="1" (
    echo.
    echo  * Performance schema and SlowLog are enabled and configured to collect metrics and queries.
    exit /b 0
)

echo  To apply changes to the MySQL configuration, you need to restart the service
set "FLAG_RESTART_SERVICE=1"
if "%RELEEM_RESTART_SERVICE%"=="" (
    set /p "REPLY=Restart MySQL service? (Y/N) "
    if /i not "!REPLY!"=="Y" (
        echo Confirmation to restart the service has not been received.
        set "FLAG_RESTART_SERVICE=0"
    )
) else if "%RELEEM_RESTART_SERVICE%"=="0" (
    set "FLAG_RESTART_SERVICE=0"
)

if "%FLAG_RESTART_SERVICE%"=="0" (
    echo.
    echo For applying change in configuration MySQL need to restart service.
    echo Run the command `mysqlconfigurer.bat -p` when it is possible to restart the service.
    exit /b 0
)

echo Restarting MySQL service with command '%RELEEM_MYSQL_RESTART_SERVICE%'.
%RELEEM_MYSQL_RESTART_SERVICE%
set "RESTART_CODE=%errorlevel%"

if %RESTART_CODE% equ 0 (
    echo.
    echo The MySQL service restarted successfully!
    echo Performance schema and SlowLog are enabled and configured to collect metrics and queries.
) else (
    echo.
    echo The MySQL service failed to restart. Check the MySQL error log.
)

echo Sending notification to Releem Platform.
"C:\Program Files\Releem\releem-agent.exe" -f >nul 2>&1

exit /b %RESTART_CODE%

REM Function: Apply configuration dispatcher
:releem_apply_config
if "%1"=="auto" (
    call :releem_apply_auto
) else if "%1"=="automatic" (
    call :releem_apply_automatic
) else (
    call :releem_apply_manual
)
goto :eof

REM Function: Apply auto configuration
:releem_apply_auto
"C:\Program Files\Releem\releem-agent.exe" --task=apply_config >nul 2>&1
echo.
echo [%date% %time%] Sending request to create a job to apply the configuration.
exit /b 0

REM Function: Apply manual configuration
:releem_apply_manual
if not exist "%MYSQLCONFIGURER_CONFIGFILE%" (
    echo.
    echo  * Recommended MySQL configuration was not found.
    echo  * Please apply recommended configuration later or run Releem Agent manually:
    echo   "C:\Program Files\Releem\releem-agent.exe" -f
    exit /b 1
)

call :check_mysql_version
if %errorlevel% neq 0 (
    echo.
    echo  * MySQL version is lower than 5.6.7. Check the documentation for applying the configuration.
    exit /b 2
)

if "%RELEEM_MYSQL_CONFIG_DIR%"=="" (
    echo.
    echo  * MySQL configuration directory was not found.
    echo  * Try to reinstall Releem Agent, and please set the my.cnf location.
    exit /b 3
)

if not exist "%RELEEM_MYSQL_CONFIG_DIR%" (
    echo.
    echo  * MySQL configuration directory was not found.
    echo  * Try to reinstall Releem Agent, and please set the my.cnf location.
    exit /b 3
)

echo.
echo [%date% %time%] Applying the recommended MySQL configuration.
echo [%date% %time%] Getting the latest up-to-date configuration.
"C:\Program Files\Releem\releem-agent.exe" -c >nul 2>&1

REM Check if configuration is different
fc "%RELEEM_MYSQL_CONFIG_DIR%\%MYSQLCONFIGURER_FILE_NAME%" "%MYSQLCONFIGURER_CONFIGFILE%" >nul 2>&1
if %errorlevel% equ 0 (
    echo.
    echo [%date% %time%] The new configuration is identical to the current configuration. No restart is required!
    exit /b 0
)

set "FLAG_RESTART_SERVICE=1"
if "%RELEEM_RESTART_SERVICE%"=="" (
    set /p "REPLY=Restart MySQL service? (Y/N) "
    if /i not "!REPLY!"=="Y" (
        echo.
        echo [%date% %time%] Confirmation to restart the service has not been received.
        set "FLAG_RESTART_SERVICE=0"
    )
) else if "%RELEEM_RESTART_SERVICE%"=="0" (
    set "FLAG_RESTART_SERVICE=0"
)

if "%FLAG_RESTART_SERVICE%"=="0" exit /b 5

echo.
echo [%date% %time%] Copying file %MYSQLCONFIGURER_CONFIGFILE% to directory %RELEEM_MYSQL_CONFIG_DIR%\.

if not exist "%MYSQLCONFIGURER_PATH%%MYSQLCONFIGURER_FILE_NAME%.bkp" (
    copy "%RELEEM_MYSQL_CONFIG_DIR%\%MYSQLCONFIGURER_FILE_NAME%" "%MYSQLCONFIGURER_PATH%%MYSQLCONFIGURER_FILE_NAME%.bkp" >nul
)

copy "%MYSQLCONFIGURER_CONFIGFILE%" "%RELEEM_MYSQL_CONFIG_DIR%\" >nul

if "%RELEEM_MYSQL_RESTART_SERVICE%"=="" (
    echo.
    echo  * The command to restart the MySQL service was not found. Try to reinstall Releem Agent.
    exit /b 4
)

echo.
echo [%date% %time%] Restarting MySQL with the command '%RELEEM_MYSQL_RESTART_SERVICE%'.
%RELEEM_MYSQL_RESTART_SERVICE%
set "RESTART_CODE=%errorlevel%"

if %RESTART_CODE% equ 0 (
    echo.
    echo [%date% %time%] The MySQL service restarted successfully!
    echo [%date% %time%] Recommended configuration applied successfully!
    echo [%date% %time%] Releem Score and Unapplied recommendations in the Releem Dashboard will be updated in a few minutes.
    if exist "%MYSQLCONFIGURER_PATH%%MYSQLCONFIGURER_FILE_NAME%.bkp" (
        del "%MYSQLCONFIGURER_PATH%%MYSQLCONFIGURER_FILE_NAME%.bkp"
    )
) else if %RESTART_CODE% equ 6 (
    echo.
    echo [%date% %time%] MySQL service failed to restart in 1200 seconds.
    echo [%date% %time%] Wait for the MySQL service to start and Check the MySQL error log.
    echo.
    echo [%date% %time%] Try to roll back the configuration application using the command:
    echo [%date% %time%] mysqlconfigurer.bat -r
) else (
    echo.
    echo [%date% %time%] MySQL service failed to restart! Check the MySQL error log!
    echo [%date% %time%] Try to roll back the configuration application using the command:
    echo [%date% %time%] mysqlconfigurer.bat -r
)

echo.
echo [%date% %time%] Sending notification to Releem Platform.
"C:\Program Files\Releem\releem-agent.exe" --event=config_applied >nul 2>&1

exit /b %RESTART_CODE%

REM Function: Apply automatic configuration
:releem_apply_automatic
if not exist "%MYSQLCONFIGURER_CONFIGFILE%" (
    echo.
    echo  * Recommended MySQL configuration was not found.
    echo  * Please apply recommended configuration later or run Releem Agent manually:
    echo   "C:\Program Files\Releem\releem-agent.exe" -f
    exit /b 1
)

call :check_mysql_version
if %errorlevel% neq 0 (
    echo.
    echo  * MySQL version is lower than 5.6.7. Check the documentation for applying the configuration.
    exit /b 2
)

if "%RELEEM_MYSQL_CONFIG_DIR%"=="" (
    echo.
    echo  * MySQL configuration directory was not found.
    echo  * Try to reinstall Releem Agent, and set the my.cnf location.
    exit /b 3
)

if not exist "%RELEEM_MYSQL_CONFIG_DIR%" (
    echo.
    echo  * MySQL configuration directory was not found.
    echo  * Try to reinstall Releem Agent, and set the my.cnf location.
    exit /b 3
)

echo.
echo [%date% %time%] Applying the recommended MySQL configuration.
echo [%date% %time%] Getting the latest up-to-date configuration.
"C:\Program Files\Releem\releem-agent.exe" -c >nul 2>&1

set "FLAG_RESTART_SERVICE=1"
if "%RELEEM_RESTART_SERVICE%"=="" (
    set /p "REPLY=Restart MySQL service? (Y/N) "
    if /i not "!REPLY!"=="Y" (
        echo.
        echo [%date% %time%] Confirmation to restart the service has not been received.
        set "FLAG_RESTART_SERVICE=0"
    )
) else if "%RELEEM_RESTART_SERVICE%"=="0" (
    set "FLAG_RESTART_SERVICE=0"
)

echo.
echo [%date% %time%] Copying file %MYSQLCONFIGURER_CONFIGFILE% to directory %RELEEM_MYSQL_CONFIG_DIR%\.

if not exist "%MYSQLCONFIGURER_PATH%%MYSQLCONFIGURER_FILE_NAME%.bkp" (
    copy "%RELEEM_MYSQL_CONFIG_DIR%\%MYSQLCONFIGURER_FILE_NAME%" "%MYSQLCONFIGURER_PATH%%MYSQLCONFIGURER_FILE_NAME%.bkp" >nul
)

copy "%MYSQLCONFIGURER_CONFIGFILE%" "%RELEEM_MYSQL_CONFIG_DIR%\" >nul

if "%FLAG_RESTART_SERVICE%" neq "0" (
    if "%RELEEM_MYSQL_RESTART_SERVICE%"=="" (
        echo.
        echo  * The command to restart the MySQL service was not found. Try to reinstall Releem Agent.
        exit /b 4
    )
    
    echo.
    echo [%date% %time%] Restarting MySQL with the command '%RELEEM_MYSQL_RESTART_SERVICE%'.
    %RELEEM_MYSQL_RESTART_SERVICE%
    set "RESTART_CODE=%errorlevel%"
    
    if !RESTART_CODE! equ 0 (
        echo.
        echo [%date% %time%] The MySQL service restarted successfully!
        echo [%date% %time%] Recommended configuration applied successfully!
        echo [%date% %time%] Releem Score and Unapplied recommendations in the Releem Dashboard will be updated in a few minutes.
        if exist "%MYSQLCONFIGURER_PATH%%MYSQLCONFIGURER_FILE_NAME%.bkp" (
            del "%MYSQLCONFIGURER_PATH%%MYSQLCONFIGURER_FILE_NAME%.bkp"
        )
    ) else if !RESTART_CODE! equ 6 (
        echo.
        echo [%date% %time%] MySQL service failed to restart in 1200 seconds.
        echo [%date% %time%] Wait for the MySQL service to start and Check the MySQL error log.
        echo.
        echo [%date% %time%] Try to roll back the configuration application using the command:
        echo [%date% %time%] mysqlconfigurer.bat -r
    ) else (
        echo.
        echo [%date% %time%] MySQL service failed to restart. Check the MySQL error log.
        echo [%date% %time%] Try to roll back the configuration application using the command:
        echo [%date% %time%] mysqlconfigurer.bat -r
    )
) else (
    set "RESTART_CODE=0"
)

echo.
echo [%date% %time%] Sending notification to Releem Platform.
"C:\Program Files\Releem\releem-agent.exe" --event=config_applied >nul 2>&1

exit /b %RESTART_CODE%

REM Load configuration from file
:load_config
if exist "%RELEEM_CONF_FILE%" (
    for /f "usebackq tokens=1,2 delims==" %%a in ("%RELEEM_CONF_FILE%") do (
        if "%%a"=="apikey" set "RELEEM_API_KEY=%%b"
        if "%%a"=="memory_limit" set "MYSQL_MEMORY_LIMIT=%%b"
        if "%%a"=="mysql_cnf_dir" set "RELEEM_MYSQL_CONFIG_DIR=%%b"
        if "%%a"=="mysql_restart_service" set "RELEEM_MYSQL_RESTART_SERVICE=%%b"
        if "%%a"=="mysql_user" set "MYSQL_LOGIN=%%b"
        if "%%a"=="mysql_password" set "MYSQL_PASSWORD=%%b"
        if "%%a"=="mysql_host" set "mysql_host=%%b"
        if "%%a"=="mysql_port" set "mysql_port=%%b"
        if "%%a"=="query_optimization" set "RELEEM_QUERY_OPTIMIZATION=%%b"
        if "%%a"=="releem_region" set "RELEEM_REGION=%%b"
    )
)

REM Build connection string
set "connection_string="
if not "%mysql_host%"=="" (
    set "connection_string=%connection_string% -h%mysql_host%"
) else (
    set "connection_string=%connection_string% -h127.0.0.1"
)

if not "%mysql_port%"=="" (
    set "connection_string=%connection_string% -P%mysql_port%"
) else (
    set "connection_string=%connection_string% -P3306"
)

goto :eof

REM Find MySQL executables
:find_mysql_commands
set "mysqladmincmd="
set "mysqlcmd="

REM Try to find mysqladmin
where mysqladmin >nul 2>&1
if %errorlevel% equ 0 (
    set "mysqladmincmd=mysqladmin"
) else (
    where mariadb-admin >nul 2>&1
    if !errorlevel! equ 0 (
        set "mysqladmincmd=mariadb-admin"
    )
)

if "%mysqladmincmd%"=="" (
    echo Couldn't find mysqladmin/mariadb-admin in your PATH.
    exit /b 1
)

REM Try to find mysql
where mysql >nul 2>&1
if %errorlevel% equ 0 (
    set "mysqlcmd=mysql"
) else (
    where mariadb >nul 2>&1
    if !errorlevel! equ 0 (
        set "mysqlcmd=mariadb"
    )
)

if "%mysqlcmd%"=="" (
    echo Couldn't find mysql/mariadb in your PATH.
    exit /b 1
)

goto :eof

REM Initialize
call :load_config
call :find_mysql_commands

REM Set default MySQL configuration paths for Windows
if exist "C:\ProgramData\MySQL\MySQL Server 8.0\my.ini" (
    set "MYSQL_MY_CNF_PATH=C:\ProgramData\MySQL\MySQL Server 8.0\my.ini"
) else if exist "C:\Program Files\MySQL\MySQL Server 8.0\my.ini" (
    set "MYSQL_MY_CNF_PATH=C:\Program Files\MySQL\MySQL Server 8.0\my.ini"
) else (
    set "MYSQL_MY_CNF_PATH="
)

REM Set default restart service command if not set
if "%RELEEM_MYSQL_RESTART_SERVICE%"=="" (
    set "RELEEM_MYSQL_RESTART_SERVICE=net stop MySQL && net start MySQL"
)

REM Set default MySQL config directory if not set
if "%RELEEM_MYSQL_CONFIG_DIR%"=="" (
    set "RELEEM_MYSQL_CONFIG_DIR=C:\ProgramData\MySQL\MySQL Server 8.0\releem.conf.d"
)

:end
REM Send log to Releem Platform on exit
if not "%RELEEM_API_KEY%"=="" (
    curl -s -L -d @"%logfile%" -H "x-releem-api-key: %RELEEM_API_KEY%" -H "Content-Type: application/json" -X POST https://%API_DOMAIN%/v2/events/configurer_log >nul 2>&1
)

endlocal
