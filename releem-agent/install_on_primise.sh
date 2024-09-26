#!/bin/bash
# install.sh - Version 1.19.3
# (C) Releem, Inc 2022
# All rights reserved

# Releem installation script: install and set up the Releem Agent on supported Linux distributions
# using the package manager.

set -e
install_script_version=1.19.3
logfile="releem-install.log"

WORKDIR="/opt/releem"
CONF="$WORKDIR/releem.conf"
MYSQL_CONF_DIR="/etc/mysql/releem.conf.d"
RELEEM_COMMAND="/bin/bash $WORKDIR/mysqlconfigurer.sh"

# Read configuration


# Set up a named pipe for logging
npipe=/tmp/$$.install.tmp
mknod $npipe p

# Log all output to a log for error checking
tee <$npipe $logfile &
exec 1>&-
exec 1>$npipe 2>&1

function on_exit() {
    rm -f $npipe
}

function on_error() {
    printf "\033[31m$ERROR_MESSAGE
It looks like you hit an issue when trying to install the Releem.

If you're still having problems, please send an email to hello@releem.com
with the contents of $logfile and we'll do our very best to help you
solve your problem.\n\033[0m\n"
}
trap on_error ERR
trap on_exit EXIT

function releem_set_cron() {
    ($sudo_cmd crontab -l 2>/dev/null | grep -v "$WORKDIR/mysqlconfigurer.sh" || true; echo "$RELEEM_CRON") | $sudo_cmd crontab -
}

function releem_update() {
    printf "\033[37m\n * Downloading latest version of Releem Agent...\033[0m\n"
    $sudo_cmd curl -w "%{http_code}" -L -o $WORKDIR/releem-agent.new https://releem.s3.amazonaws.com/v2/releem-agent-$(arch)
    $sudo_cmd curl -w "%{http_code}" -L -o $WORKDIR/mysqlconfigurer.sh.new https://releem.s3.amazonaws.com/v2/mysqlconfigurer.sh
    $sudo_cmd $WORKDIR/releem-agent  stop || true
    $sudo_cmd mv $WORKDIR/releem-agent.new $WORKDIR/releem-agent
    $sudo_cmd mv $WORKDIR/mysqlconfigurer.sh.new $WORKDIR/mysqlconfigurer.sh
    $sudo_cmd chmod 755 $WORKDIR/mysqlconfigurer.sh   $WORKDIR/releem-agent
    $sudo_cmd $WORKDIR/releem-agent start || true
    $sudo_cmd $WORKDIR/releem-agent -f
    
    echo
    echo
    echo -e "Releem Agent updated successfully."
    echo
    echo -e "To check MySQL Performance Score please visit https://app.releem.com/dashboard?menu=metrics"
    echo

    exit 0
}

if [ "$0" == "uninstall" ];
then
    trap - EXIT
    $WORKDIR/releem-agent --event=agent_uninstall > /dev/null
    printf "\033[37m\n * Configure crontab\033[0m\n"
    ($sudo_cmd crontab -l 2>/dev/null | grep -v "$WORKDIR/mysqlconfigurer.sh" || true) | $sudo_cmd crontab -
    printf "\033[37m\n * Stoping Releem Agent service...\033[0m\n"
    releem_agent_stop=$($sudo_cmd $WORKDIR/releem-agent  stop)
    if [ $? -eq 0 ]; then
        printf "\033[32m\n Stop Releem Agent successfuly\033[0m\n"
    else
        echo $releem_agent_stop
        printf "\033[31m\n Restart Releem Agent failed\033[0m\n"
    fi
    printf "\033[37m\n * Uninstalling Releem Agent service...\033[0m\n"
    releem_agent_remove=$($sudo_cmd $WORKDIR/releem-agent remove)
    if [ $? -eq 0 ]; then
        printf "\033[32m\n Uninstall Releem Agent successfuly\033[0m\n"
    else
        echo $releem_agent_remove
        printf "\033[31m\n Reinstall Releem Agent failed\033[0m\n"
    fi
    printf "\033[37m\n * Remove files Releem Agent\033[0m\n"
    $sudo_cmd rm -rf $WORKDIR
    exit 0
fi

apikey=
if [ -n "$RELEEM_API_KEY" ]; then
    apikey=$RELEEM_API_KEY
fi

if [ ! "$apikey" ]; then
    printf "\033[31mReleem API key is not available in RELEEM_API_KEY environment variable. Please sigh up at https://releem.com\033[0m\n"
    exit 1;
fi

connection_string=""  
root_connection_string=""
if [ -n "$RELEEM_MYSQL_HOST" ]; then
    if [ -S "$RELEEM_MYSQL_HOST" ]; then
        mysql_user_host="localhost"
        connection_string="${connection_string} --socket=${RELEEM_MYSQL_HOST}"
        root_connection_string="${root_connection_string} --socket=${RELEEM_MYSQL_HOST}"
    else
        if [ "$RELEEM_MYSQL_HOST" == "127.0.0.1" ]; then
            mysql_user_host="127.0.0.1"
        else
            mysql_user_host="%"
        fi
        connection_string="${connection_string} --host=${RELEEM_MYSQL_HOST}"
    fi
else
    mysql_user_host="127.0.0.1"
    connection_string="${connection_string} --host=127.0.0.1"
fi

if [ -n "$RELEEM_MYSQL_PORT" ]; then
    connection_string="${connection_string} --port=${RELEEM_MYSQL_PORT}"
else
    connection_string="${connection_string} --port=3306"
fi



# Root user detection
if [ "$(echo "$UID")" = "0" ]; then
    sudo_cmd=''
else
    sudo_cmd='sudo'
fi

# Parse parameters
while getopts "u" option
do
case "${option}"
in
u) releem_update;;
esac
done

# OS/Distro Detection
# Try lsb_release, fallback with /etc/issue then uname command
KNOWN_DISTRIBUTION="(Debian|Ubuntu|RedHat|CentOS|Amazon)"
DISTRIBUTION=$(lsb_release -d 2>/dev/null | grep -Eo $KNOWN_DISTRIBUTION  || grep -Eo $KNOWN_DISTRIBUTION /etc/issue 2>/dev/null || grep -Eo $KNOWN_DISTRIBUTION /etc/Eos-release 2>/dev/null || grep -m1 -Eo $KNOWN_DISTRIBUTION /etc/os-release 2>/dev/null || uname -s)

if [ -f /etc/debian_version ] || [ "$DISTRIBUTION" == "Debian" ] || [ "$DISTRIBUTION" == "Ubuntu" ]; then
    OS="Debian"
elif [ -f /etc/redhat-release ] || [ "$DISTRIBUTION" == "RedHat" ] || [ "$DISTRIBUTION" == "CentOS" ] || [ "$DISTRIBUTION" == "Amazon" ]; then
    OS="RedHat"
# Some newer distros like Amazon may not have a redhat-release file
elif [ -f /etc/system-release ] || [ "$DISTRIBUTION" == "Amazon" ]; then
    OS="RedHat"
# Arista is based off of Fedora14/18 but do not have /etc/redhat-release
elif [ -f /etc/Eos-release ] || [ "$DISTRIBUTION" == "Arista" ]; then
    OS="RedHat"
fi

# Install the necessary package sources
# if [ "$OS" = "RedHat" ]; then
#     echo -e "\033[37m\n * Installing dependencies...\n\033[0m"
#     if [ -x "/usr/bin/dnf" ]; then
#         package_manager='dnf'
#     else
#         package_manager='yum'
#     fi
#     which curl &> /dev/null || $sudo_cmd $package_manager -y install curl
# elif [ "$OS" = "Debian" ]; then
#     printf "\033[37m\n * Installing dependences...\n\033[0m\n"
#     which curl &> /dev/null || ($sudo_cmd apt-get update ; $sudo_cmd apt-get install -y --force-yes curl)
# else
#     printf "\033[31mYour OS or distribution are not supported by this install script.\033[0m\n"
#     exit;
# fi

$sudo_cmd rm -rf $WORKDIR
# Create work directory
if [ ! -e $CONF ]; then
    $sudo_cmd mkdir -p $WORKDIR
    $sudo_cmd mkdir -p $WORKDIR/conf
fi

printf "\033[37m\n * Downloading Releem Agent, architecture $(arch)...\033[0m\n"
$sudo_cmd cp -f mysqlconfigurer.sh $WORKDIR/mysqlconfigurer.sh
$sudo_cmd cp -f releem-agent-$(arch) $WORKDIR/releem-agent


$sudo_cmd chmod 755 $WORKDIR/mysqlconfigurer.sh $WORKDIR/releem-agent


printf "\033[37m\n * Configure the application...\033[0m\n"
printf "\033[37m\n * Detected service name for appling config\033[0m\n"
systemctl_cmd=$(which systemctl || true)
if [ -n "$systemctl_cmd" ];then
    # Check if MySQL is running
    if $sudo_cmd $systemctl_cmd status mysql >/dev/null 2>&1; then
        service_name_cmd="$sudo_cmd $systemctl_cmd restart mysql"
    elif $sudo_cmd $systemctl_cmd status mysqld >/dev/null 2>&1; then
        service_name_cmd="$sudo_cmd $systemctl_cmd restart mysqld"
    elif $sudo_cmd $systemctl_cmd status mariadb >/dev/null 2>&1; then
        service_name_cmd="$sudo_cmd $systemctl_cmd restart mariadb"
    else
        printf "\033[31m\n * Failed to determine service to restart. The automatic applying configuration will not work. \n\033[0m"
    fi
else
    # Check if MySQL is running
    if [ -f /etc/init.d/mysql ]; then
        service_name_cmd="$sudo_cmd /etc/init.d/mysql restart"
    elif [ -f /etc/init.d/mysqld ]; then
        service_name_cmd="$sudo_cmd /etc/init.d/mysqld restart"
    elif [ -f /etc/init.d/mariadb ]; then
        service_name_cmd="$sudo_cmd /etc/init.d/mariadb restart"
    else
        printf "\033[31m\n * Failed to determine service to restart. The automatic applying configuration will not work. \n\033[0m"
    fi
fi

printf "\033[37m\n * Configure catalog for copy recommend config\033[0m\n"
if [[ -n $RELEEM_MYSQL_MY_CNF_PATH ]];
then
	MYSQL_MY_CNF_PATH=$RELEEM_MYSQL_MY_CNF_PATH
else
	if [ -f "/etc/my.cnf" ]; then
		MYSQL_MY_CNF_PATH="/etc/my.cnf"
	elif [ -f "/etc/mysql/my.cnf" ]; then
		MYSQL_MY_CNF_PATH="/etc/mysql/my.cnf"
	else
		read -p "File my.cnf not found in default path. Please set the current location of the configuration file: " -r
		echo    # move to a new line
		MYSQL_MY_CNF_PATH=$REPLY
	fi
fi
if [ ! -f "$MYSQL_MY_CNF_PATH" ]; then
	printf "\033[31m * File $MYSQL_MY_CNF_PATH not found. The automatic applying configuration is disabled. Please, reinstall the Releem Agent.\033[0m\n"
else
	# FLAG_APPLY_CHANGE=0
	# if [[ -z $RELEEM_MYSQL_MY_CNF_PATH ]];
	# then
	#     	read -p "Please confirm MySQL configuration location $MYSQL_MY_CNF_PATH? (Y/N) " -n 1 -r
	#     	echo    # move to a new line
	#     	if [[ $REPLY =~ ^[Yy]$ ]]
	#     	then
	# 		       FLAG_APPLY_CHANGE=1
	# 	    else
	# 		       FLAG_APPLY_CHANGE=0
	# 		       printf "\033[31m\n * A confirmation has not been received. The automatic applying configuration is disabled. Please, reinstall the Releem Agent.\033[0m\n"
	# 	    fi
	# else
	# 	    FLAG_APPLY_CHANGE=1
	# fi
	# if [ $FLAG_APPLY_CHANGE -eq 1 ];
	# then

    printf "\033[37m\n * The $MYSQL_MY_CNF_PATH file is used for automatic Releem settings. \n\033[0m"
	printf "\033[37m\n * Adding directive includedir to the MySQL configuration $MYSQL_MY_CNF_PATH.\n\033[0m"
	$sudo_cmd mkdir -p $MYSQL_CONF_DIR
    $sudo_cmd chmod 755 $MYSQL_CONF_DIR
    #Исключить дублирование
    if [ `$sudo_cmd grep -cE "!includedir $MYSQL_CONF_DIR" $MYSQL_MY_CNF_PATH` -eq 0 ];
	then
	    echo -e "\n!includedir $MYSQL_CONF_DIR" | $sudo_cmd tee -a $MYSQL_MY_CNF_PATH >/dev/null
	fi
	# fi
fi


printf "\033[37m\n * Configure MySQL user for collect data\033[0m\n"
FLAG_SUCCESS=0
if [ -n "$RELEEM_MYSQL_PASSWORD" ] && [ -n "$RELEEM_MYSQL_LOGIN" ]; then
    printf "\033[37m\n * Using MySQL login and password from environment variables\033[0m\n"
    FLAG_SUCCESS=1
#elif [ -n "$RELEEM_MYSQL_ROOT_PASSWORD" ]; then
else
    printf "\033[37m\n * Using MySQL root user\033[0m\n"
    if [[ $(mysqladmin ${root_connection_string} --user=root --password=${RELEEM_MYSQL_ROOT_PASSWORD} ping 2>/dev/null || true) == "mysqld is alive" ]];
    then
        printf "\033[37m\n Connect to MySQL - successful \033[0m\n"
        RELEEM_MYSQL_LOGIN="releem"
        RELEEM_MYSQL_PASSWORD=$(cat /dev/urandom | tr -cd '%*)?@#~' | head -c2 ; cat /dev/urandom | tr -cd '%*)?@#~A-Za-z0-9%*)?@#~' | head -c16 ; cat /dev/urandom | tr -cd '%*)?@#~' | head -c2 )
        mysql  ${root_connection_string} --user=root --password=${RELEEM_MYSQL_ROOT_PASSWORD} -Be "DROP USER '${RELEEM_MYSQL_LOGIN}'@'${mysql_user_host}' ;" 2>/dev/null || true
        mysql  ${root_connection_string} --user=root --password=${RELEEM_MYSQL_ROOT_PASSWORD} -Be "CREATE USER '${RELEEM_MYSQL_LOGIN}'@'${mysql_user_host}' identified by '${RELEEM_MYSQL_PASSWORD}';"
        mysql  ${root_connection_string} --user=root --password=${RELEEM_MYSQL_ROOT_PASSWORD} -Be "GRANT PROCESS ON *.* TO '${RELEEM_MYSQL_LOGIN}'@'${mysql_user_host}';"
        mysql  ${root_connection_string} --user=root --password=${RELEEM_MYSQL_ROOT_PASSWORD} -Be "GRANT REPLICATION CLIENT ON *.* TO '${RELEEM_MYSQL_LOGIN}'@'${mysql_user_host}';"
        mysql  ${root_connection_string} --user=root --password=${RELEEM_MYSQL_ROOT_PASSWORD} -Be "GRANT SHOW VIEW ON *.* TO '${RELEEM_MYSQL_LOGIN}'@'${mysql_user_host}';"
        if mysql  ${root_connection_string} --user=root --password=${RELEEM_MYSQL_ROOT_PASSWORD} -Be "GRANT SELECT ON performance_schema.events_statements_summary_by_digest TO '${RELEEM_MYSQL_LOGIN}'@'${mysql_user_host}';" 
        then
            echo "Successfully GRANT" > /dev/null
        else
            printf "\033[31m\n This database version is too old, and it doesn’t collect SQL Queries Latency metrics. You couldn’t see Latency in the Dashboard.\033[0m\n"
        fi
        #mysql  ${root_connection_string} --user=root --password=${RELEEM_MYSQL_ROOT_PASSWORD} -Be "GRANT SELECT, PROCESS,EXECUTE, REPLICATION CLIENT,SHOW DATABASES,SHOW VIEW ON *.* TO '${RELEEM_MYSQL_LOGIN}'@'${mysql_user_host}';"
        printf "\033[32m\n Created new user \`${RELEEM_MYSQL_LOGIN}\`\033[0m\n"
        FLAG_SUCCESS=1
    else
        printf "\033[31m\n MySQL connection failed with user root with error:\033[0m\n"
        mysqladmin ${root_connection_string} --user=root --password=${RELEEM_MYSQL_ROOT_PASSWORD} ping || true
        printf "\033[31m\n%s\033[0m\n" "Check that the password is correct, the execution of the command \`mysqladmin ${root_connection_string} --user=root --password=<MYSQL_ROOT_PASSWORD> ping\` and reinstall the agent."
        exit 1
    fi
#else
#    printf "\033[31m\n Variable RELEEM_MYSQL_ROOT_PASSWORD not found.\n Please, reinstall the agent by setting the \"RELEEM_MYSQL_ROOT_PASSWORD\" variable\033[0m\n"
#    exit 1
fi

if [ "$FLAG_SUCCESS" == "1" ]; then
    if [[ $(mysqladmin ${connection_string} --user=${RELEEM_MYSQL_LOGIN} --password=${RELEEM_MYSQL_PASSWORD} ping 2>/dev/null || true) == "mysqld is alive" ]];
    then
        printf "\033[32m\n Connecting to MySQL with user \`${RELEEM_MYSQL_LOGIN}\` - successfull \033[0m\n"
        MYSQL_LOGIN=$RELEEM_MYSQL_LOGIN
        MYSQL_PASSWORD=$RELEEM_MYSQL_PASSWORD
    else
        printf "\033[31m\n Connect to mysql failed with user \`${RELEEM_MYSQL_LOGIN}\` with error:\033[0m\n"
        mysqladmin ${connection_string} --user=${RELEEM_MYSQL_LOGIN} --password=${RELEEM_MYSQL_PASSWORD} ping || true
        printf "\033[31m\n%s\033[0m\n" "Check that the user and password is correct, the execution of the command \`mysqladmin ${connection_string} --user=${RELEEM_MYSQL_LOGIN} --password=${RELEEM_MYSQL_PASSWORD} ping\` and reinstall the agent."
        exit 1
    fi
fi



# printf "\033[37m\n * Checking ~/.my.cnf...\033[0m\n"
# if [ ! -e ~/.my.cnf ]; then
#     printf "\033[37m\n * Please create ~/.my.cnf file with the following content:\033[0m\n"
#     echo -e ""
#     echo -e "[client]"
#     echo -e "user=root"
#     echo -e "password=[your MySQL root password]"
#     echo -e ""
#     read -p "Are you ready to proceed? (Y/N) " -n 1 -r
#     echo    # move to a new line
#     if [[ $REPLY =~ ^[Nn]$ ]]
#     then
#         exit 1
#     fi
# fi


printf "\033[37m\n * Configure mysql memory limit\033[0m\n"
if [ -n "$RELEEM_MYSQL_MEMORY_LIMIT" ]; then

    if [ "$RELEEM_MYSQL_MEMORY_LIMIT" -gt 0 ]; then
        MYSQL_LIMIT=$RELEEM_MYSQL_MEMORY_LIMIT
    fi
else
    echo
    printf "\033[37m\n In case you are using MySQL in Docker or it isn't dedicated server for MySQL.\033[0m\n"
    read -p "Should we limit MySQL memory? (Y/N) " -n 1 -r
    echo    # move to a new line
    if [[ $REPLY =~ ^[Yy]$ ]]
    then
        read -p "Please set MySQL Memory Limit (megabytes):" -r
        echo    # move to a new line
        MYSQL_LIMIT=$REPLY
    fi
fi

printf "\033[37m\n * Saving variables to Releem Agent configuration\033[0m\n"

printf "\033[37m\n - Adding API key to the Releem Agent configuration: $CONF\n\033[0m"
echo "apikey=\"$apikey\"" | $sudo_cmd tee -a $CONF >/dev/null

printf "\033[37m - Adding Releem Configuration Directory $WORKDIR/conf to Releem Agent configuration: $CONF\n\033[0m"
echo "releem_cnf_dir=\"$WORKDIR/conf\"" | $sudo_cmd tee -a $CONF >/dev/null

if [ -n "$MYSQL_LOGIN" ] && [ -n "$MYSQL_PASSWORD" ]; then
    printf "\033[37m - Adding user and password mysql to the Releem Agent configuration: $CONF\n\033[0m"
	echo "mysql_user=\"$MYSQL_LOGIN\"" | $sudo_cmd tee -a $CONF >/dev/null
	echo "mysql_password=\"$MYSQL_PASSWORD\"" | $sudo_cmd tee -a $CONF >/dev/null
fi
if [ -n "$RELEEM_MYSQL_HOST" ]; then
    printf "\033[37m - Adding MySQL host to the Releem Agent configuration: $CONF\n\033[0m"
	echo "mysql_host=\"$RELEEM_MYSQL_HOST\"" | $sudo_cmd tee -a $CONF >/dev/null
fi
if [ -n "$RELEEM_MYSQL_PORT" ]; then
    printf "\033[37m - Adding MySQL port to the Releem Agent configuration: $CONF\n\033[0m"
	echo "mysql_port=\"$RELEEM_MYSQL_PORT\"" | $sudo_cmd tee -a $CONF >/dev/null
fi
if [ -n "$MYSQL_LIMIT" ]; then
    printf "\033[37m - Adding Memory Limit to the Releem Agent configuration: $CONF\n\033[0m"
	echo "memory_limit=\"$MYSQL_LIMIT\"" | $sudo_cmd tee -a $CONF >/dev/null
fi
if [ -n "$service_name_cmd" ]; then
    printf "\033[37m - Adding MySQL restart command to the Releem Agent configuration: $CONF\n\033[0m"
	echo "mysql_restart_service=\"$service_name_cmd\"" | $sudo_cmd tee -a $CONF >/dev/null
fi
if [ -d "$MYSQL_CONF_DIR" ]; then
	printf "\033[37m - Adding MySQL include directory to the Releem Agent configuration $CONF.\n\033[0m"
	echo "mysql_cnf_dir=\"$MYSQL_CONF_DIR\"" | $sudo_cmd tee -a $CONF >/dev/null
fi
if [ -n "$RELEEM_HOSTNAME" ]; then
    printf "\033[37m - Adding hostname to the Releem Agent configuration: $CONF\n\033[0m"
	echo "hostname=\"$RELEEM_HOSTNAME\"" | $sudo_cmd tee -a $CONF >/dev/null
else
    RELEEM_HOSTNAME=$(hostname 2>&1)
    if [ $? -eq 0 ];
    then
        printf "\033[37m - Adding autodetected hostname to the Releem Agent configuration: $CONF\n\033[0m"
	    echo "hostname=\"$RELEEM_HOSTNAME\"" | $sudo_cmd tee -a $CONF >/dev/null        
    else
        printf "\033[31m The variable RELEEM_HOSTNAME is not defined and the hostname could not be determined automatically with error\033[0m\n $RELEEM_HOSTNAME.\n\033[0m"
    fi
fi
if [ -n "$RELEEM_ENV" ]; then
	echo "env=\"$RELEEM_ENV\"" | $sudo_cmd tee -a $CONF >/dev/null
fi
if [ -n "$RELEEM_DEBUG" ]; then
	echo "debug=$RELEEM_DEBUG" | $sudo_cmd tee -a $CONF >/dev/null
fi
echo "interval_seconds=60" | $sudo_cmd tee -a $CONF >/dev/null
echo "interval_read_config_seconds=3600" | $sudo_cmd tee -a $CONF >/dev/null

# Secure the configuration file
$sudo_cmd chmod 640 $CONF


printf "\033[37m\n * Configure crontab...\033[0m\n"
RELEEM_CRON="00 00 * * * PATH=/bin:/sbin:/usr/bin:/usr/sbin $RELEEM_COMMAND -u"
if [ -z "$RELEEM_CRON_ENABLE" ]; then
    printf "\033[37m Please add the following string in crontab to get recommendations:\033[0m\n"
    printf "\033[32m$RELEEM_CRON\033[0m\n\n"
    read -p "Can we do it automatically? (Y/N) " -n 1 -r
    echo    # move to a new line
    if [[ $REPLY =~ ^[Yy]$ ]]
    then
        releem_set_cron
    fi
elif [ "$RELEEM_CRON_ENABLE" -gt 0 ]; then
    releem_set_cron
fi

set +e
trap - ERR
if [ -z "$RELEEM_AGENT_DISABLE" ]; then
    # First run of Releem Agent to check MySQL Performance Score
    printf "\033[37m\n * Executing Releem Agent for first time...\033[0m\n"
    $sudo_cmd $WORKDIR/releem-agent -f
    $sudo_cmd timeout 3 $WORKDIR/releem-agent
fi
printf "\033[37m\n * Installing and starting Releem Agent service to collect metrics..\033[0m\n"
releem_agent_remove=$($sudo_cmd $WORKDIR/releem-agent remove)
releem_agent_install=$($sudo_cmd $WORKDIR/releem-agent install)
if [ $? -eq 0 ]; then
    printf "\033[32m\n Installing Releem Agent - successful\033[0m\n"
else
    echo $releem_agent_remove
    echo $releem_agent_install
    printf "\033[31m\n Installing Releem Agent - failed\033[0m\n"
fi
releem_agent_stop=$($sudo_cmd $WORKDIR/releem-agent  stop)
releem_agent_start=$($sudo_cmd $WORKDIR/releem-agent  start)
if [ $? -eq 0 ]; then
    printf "\033[32m\n Restarting Releem Agent - successful\033[0m\n"
else
    echo $releem_agent_stop
    echo $releem_agent_start
    printf "\033[31m\n Restarting Releem Agent - failed\033[0m\n"
fi
# $sudo_cmd $WORKDIR/releem-agent  status
# if [ $? -eq 0 ]; then
#     echo "Status successfull"
# else
#     echo "remove failes"
# fi
trap on_error ERR
set -e
sleep 3
releem_agent_pid=$(pgrep releem-agent || true)
if [ -z "$releem_agent_pid" ]; then
    printf "\033[31m\n The releem-agent process was not found! Check the system log for an error.\033[0m\n"
    on_error
    exit 1;
fi
# Enable perfomance schema
$sudo_cmd $RELEEM_COMMAND -p

printf "\033[37m\n\033[0m"
printf "\033[37m * Releem Agent is successfully installed.\033[0m\n"
printf "\033[37m\n\033[0m"
printf "\033[37m * To view Releem recommendations and MySQL metrics, visit https://app.releem.com/dashboard\033[0m"
printf "\033[37m\n\033[0m"
