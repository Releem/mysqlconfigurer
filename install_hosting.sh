#!/bin/bash
# install.sh - Version 0.9.6
# (C) Releem, Inc 2022
# All rights reserved

# Releem installation script: install and set up the Releem Agent on supported Linux distributions
# using the package manager.

set -e
install_script_version=0.9.6
logfile="releem-install.log"

WORKDIR="/opt/releem"
CONF="$WORKDIR/releem.conf"
MYSQL_CONF_DIR="/etc/mysql/releem.conf.d"
# Read configuration


# Set up a named pipe for logging
npipe=/tmp/$$.tmp
mknod $npipe p

# Log all output to a log for error checking
tee <$npipe $logfile &
exec 1>&-
exec 1>$npipe 2>&1
trap 'rm -f $npipe' EXIT

function on_error() {
    printf "\033[31m$ERROR_MESSAGE
It looks like you hit an issue when trying to install the Releem.

If you're still having problems, please send an email to support@releem.com
with the contents of releem-install.log and we'll do our very best to help you
solve your problem.\n\033[0m\n"
}
trap on_error ERR

function releem_set_cron() {
    ($sudo_cmd crontab -l 2>/dev/null | grep -v "$WORKDIR/mysqlconfigurer.sh" || true; echo "$RELEEM_CRON") | $sudo_cmd crontab -
}

function releem_update() {
    printf "\033[34m\n* Downloading latest version of Releem Agent...\033[0m\n"
    $sudo_cmd curl -s -L -o $WORKDIR/mysqlconfigurer.sh https://releem.s3.amazonaws.com/mysqlconfigurer_hosting.sh

    echo
    echo
    echo -e "Releem Agent updated successfully."
    echo
    echo -e "To check MySQL Performance Score please visit https://app.releem.com/dashboard?menu=metrics"
    echo

    exit 0
}


apikey=
if [ -n "$RELEEM_API_KEY" ]; then
    apikey=$RELEEM_API_KEY
fi

if [ ! "$apikey" ]; then
    printf "\033[31mReleem API key is not available in RELEEM_API_KEY environment variable. Please sigh up at https://releem.com\033[0m\n"
    exit 1;
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
if [ "$OS" = "RedHat" ]; then
    echo -e "\033[34m\n* Installing dependencies...\n\033[0m"

    if [ -x "/usr/bin/dnf" ]; then
        package_manager='dnf'
    else
        package_manager='yum'
    fi

    $sudo_cmd $package_manager -y install net-tools perl-JSON perl-Data-Dumper perl-Getopt-Long

elif [ "$OS" = "Debian" ]; then
    printf "\033[34m\n* Installing dependences...\n\033[0m\n"

    $sudo_cmd apt-get update
    $sudo_cmd apt-get install -y --force-yes curl net-tools libjson-perl

else
    printf "\033[31mYour OS or distribution are not supported by this install script.\033[0m\n"
    exit;
fi

$sudo_cmd rm -rf $WORKDIR
# Create work directory
if [ ! -e $CONF ]; then
    $sudo_cmd mkdir -p $WORKDIR
    $sudo_cmd mkdir -p $WORKDIR/conf
fi

printf "\033[34m\n* Downloading Releem Agent...\033[0m\n"
$sudo_cmd curl -o $WORKDIR/mysqlconfigurer.sh https://releem.s3.amazonaws.com/mysqlconfigurer_hosting.sh

printf "\033[34m\n* Configure the application to use the Releem recommended configuration...\033[0m\n"

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
        printf "\033[31m\n* Failed to determine service to restart. The automatic applying configuration will not work. \n\033[0m"
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
        printf "\033[31m\n* Failed to determine service to restart. The automatic applying configuration will not work. \n\033[0m"
    fi
fi
if [ -n "$service_name_cmd" ]; then
    printf "\033[34m\n* Adding MySQL restart command to the Releem Agent configuration: $CONF\n\033[0m"
    echo "mysql_restart_service=\"$service_name_cmd\"" | $sudo_cmd tee -a $CONF >/dev/null
fi

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
	printf "\033[31m* File $MYSQL_MY_CNF_PATH not found. The automatic applying configuration is disabled. Please, reinstall the Releem Agent.\033[0m\n"
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
	# 		       printf "\033[31m\n* A confirmation has not been received. The automatic applying configuration is disabled. Please, reinstall the Releem Agent.\033[0m\n"
	# 	    fi
	# else
	# 	    FLAG_APPLY_CHANGE=1
	# fi
	# if [ $FLAG_APPLY_CHANGE -eq 1 ];
	# then

    printf "\033[34m\n* The $MYSQL_MY_CNF_PATH file is used for automatic Releem settings. \n\033[0m"

		printf "\033[34m\n* Adding MySQL configuration directory to the Releem Agent configuration $CONF.\n\033[0m"
		echo "mysql_cnf_dir=$MYSQL_CONF_DIR" | $sudo_cmd tee -a $CONF >/dev/null

		printf "\033[34m\n* Adding directive includedir to the MySQL configuration $MYSQL_MY_CNF_PATH.\n\033[0m"
		$sudo_cmd mkdir -p $MYSQL_CONF_DIR
        #Исключить дублирование
        if [ `grep -cE "!includedir $MYSQL_CONF_DIR" $MYSQL_MY_CNF_PATH` -eq 0 ];
		    then
		        echo "!includedir $MYSQL_CONF_DIR" | $sudo_cmd tee -a $MYSQL_MY_CNF_PATH >/dev/null
		    fi
	# fi
fi


printf "\033[34m\n* Checking /root/.my.cnf...\033[0m\n"
if $sudo_cmd [ ! -e /root/.my.cnf ]; then
    printf "\033[34m\n* Please create /root/.my.cnf file with the following content:\033[0m\n"
    echo -e ""
    echo -e "[client]"
    echo -e "user=root"
    echo -e "password=[your MySQL root password]"
    echo -e ""
    read -p "Are you ready to proceed? (Y/N) " -n 1 -r
    echo    # move to a new line
    if [[ $REPLY =~ ^[Nn]$ ]]
    then
        exit 1
    fi
fi

RELEEM_COMMAND="/bin/bash $WORKDIR/mysqlconfigurer.sh"

if [ -n "$RELEEM_MYSQL_MEMORY_LIMIT" ]; then

    if [ "$RELEEM_MYSQL_MEMORY_LIMIT" -gt 0 ]; then
        MYSQL_LIMIT=$RELEEM_MYSQL_MEMORY_LIMIT
    fi
else
    echo
    printf "\033[34m\n* In case you are using MySQL in Docker or it isn't dedicated server for MySQL.\033[0m\n"
    read -p "Should we limit MySQL memory? (Y/N) " -n 1 -r
    echo    # move to a new line
    if [[ $REPLY =~ ^[Yy]$ ]]
    then
        read -p "Please set MySQL Memory Limit (megabytes):" -r
        echo    # move to a new line
        MYSQL_LIMIT=$REPLY
    fi
fi

# Create configuration file
printf "\033[34m\n* Adding API key to the Releem Agent configuration: $CONF\n\033[0m\n"
echo "apikey=$apikey" | $sudo_cmd tee -a  $CONF >/dev/null
printf "\033[34m\n* Adding Releem Configuration Directory $WORKDIR/conf to Releem Agent configuration: $CONF\n\033[0m\n"
echo "releem_cnf_dir=$WORKDIR/conf" | $sudo_cmd tee -a $CONF >/dev/null

if [ -n "$MYSQL_LIMIT" ]; then
    printf "\033[34m\n* Adding Memory Limit to the Releem Agent configuration: $CONF\n\033[0m\n"
    echo "memory_limit=$MYSQL_LIMIT" | $sudo_cmd tee -a $CONF >/dev/null
fi

# Secure the configuration file
$sudo_cmd chmod 640 $CONF

if [ -z "$RELEEM_AGENT_DISABLE" ]; then
    # First run of Releem Agent to check MySQL Performance Score
    printf "\033[34m\n* Executing Releem Agent for first time...\033[0m\n"
    $sudo_cmd $RELEEM_COMMAND
fi

RELEEM_CRON="* * * * * PATH=/bin:/sbin:/usr/bin:/usr/sbin $RELEEM_COMMAND -c"

if [ -z "$RELEEM_CRON_ENABLE" ]; then
    printf "\033[34m* Please add the following string in crontab to get recommendations:\033[0m\n"
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

printf "\033[34m\n\033[0m"
printf "\033[34m* To run Releem Agent manually please use the following command:\033[0m\n"
printf "\033[32m$RELEEM_COMMAND\033[0m\n\n"
printf "\033[34m* To check MySQL Performance Score please visit https://app.releem.com/dashboard?menu=metrics\033[0m"
printf "\033[34m\n\033[0m"
