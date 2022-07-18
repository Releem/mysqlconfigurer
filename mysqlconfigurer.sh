#!/bin/bash
# install.sh - Version 0.9.0
# (C) Releem, Inc 2022
# All rights reserved

# Variables
MYSQLCONFIGURER_PATH="/tmp/.mysqlconfigurer/"
RELEEM_CONF_FILE="/opt/releem/releem.conf"
MYSQLTUNER_FILENAME=$MYSQLCONFIGURER_PATH"mysqltuner.pl"
MYSQLTUNER_REPORT=$MYSQLCONFIGURER_PATH"mysqltunerreport.json"
MYSQLCONFIGURER_CONFIGFILE=$MYSQLCONFIGURER_PATH"z_aiops_mysql.cnf"
MYSQL_MEMORY_LIMIT=0

function releem_apply_config() {
    printf "\033[34m\n* Applying config of Releem Agent...\033[0m\n"
    if [ ! -f $MYSQLCONFIGURER_CONFIGFILE ]; then
        printf "\033[34m\nNot found mysql config file.\033[0m"
        printf "\033[34m* To run Releem Agent manually please use the following command:\033[0m\n"
        printf "\033[32m bash /opt/releem/mysqlconfigurer.sh \033[0m\n\n"
        printf "\033[34m\n\033[0m"
        exit 1;
    fi
    if [ -z "$RELEEM_CONFIG_DIR" ]; then
        printf "\033[34m\nNot found releem config file.\033[0m"
        printf "\033[34m* Try reinstalled Releem Agent and please settings path to my.cnf \033[0m\n"
        printf "\033[32m bash /opt/releem/mysqlconfigurer.sh \033[0m\n\n"
        printf "\033[34m\n\033[0m"
        exit 1;
    fi
    echo "Copy $MYSQLCONFIGURER_CONFIGFILE to catalog $RELEEM_CONFIG_DIR/ "
    cp $MYSQLCONFIGURER_CONFIGFILE $RELEEM_CONFIG_DIR/
    echo "Test config"

    # Root user detection
    if [ "$(echo "$UID")" = "0" ]; then
        sudo_cmd=''
    else
        sudo_cmd='sudo'
    fi

    service_cmd=$(which systemctl)

    if [ -n "$service_cmd" ];then
        # Check if MySQL is running
        if $sudo_cmd $service_cmd is-active --quiet mysql; then
            echo -e "mysql Service is running already. Nothing to do here.\n"
            service_name_cmd="$sudo_cmd $service_cmd restart mysql"
        elif $sudo_cmd $service_cmd is-active --quiet mysqld; then
            echo -e "mysqld Service is running already. Nothing to do here.\n"
            service_name_cmd="$sudo_cmd $service_cmd restart mysqld"
        elif $sudo_cmd $service_cmd is-active --quiet mariadb; then
            echo -e "mariadb Service is running already. Nothing to do here.\n"
            service_name_cmd="$sudo_cmd $service_cmd restart mariadb"
        fi
    else
        # Check if MySQL is running
        if [ -f /etc/init.d/mysql ]; then
            echo -e "mysql init is running already. Nothing to do here.\n"
            service_name_cmd="$sudo_cmd /etc/init.d/mysql restart"
        elif [ -f /etc/init.d/mysqld ]; then
            echo -e "mysqld init is running already. Nothing to do here.\n"
            service_name_cmd="$sudo_cmd /etc/init.d/mysqld restart"
        elif [ -f /etc/init.d/mariadb ]; then
            echo -e "mariadb init is running already. Nothing to do here.\n"
            service_name_cmd="$sudo_cmd /etc/init.d/mariadb restart"
        fi
    fi
    echo "$service_name_cmd"

    read -p "Confirm restarted mysql service? (Y/N) " -n 1 -r
    echo    # move to a new line

    if [[ $REPLY =~ ^[Yy]$ ]]
    then
        echo "Restarting..."
        eval "$service_name_cmd"
    fi

    sleep 2
    if [[ $(mysqladmin ping) == "mysqld is alive" ]];
    then
        echo "Mysql server start after 10 seconds"
    else
        echo "Mysql server not start after 10 seconds. Check mysql error log."
    fi
    exit 0
}


if test -f $RELEEM_CONF_FILE ; then
    . $RELEEM_CONF_FILE

    RELEEM_API_KEY=$apikey
    if [ ! -z $memory_limit ]; then
        MYSQL_MEMORY_LIMIT=$memory_limit
    fi
    if [ ! -z $mysql_cnf_dir ]; then
        RELEEM_CONFIG_DIR=$mysql_cnf_dir
    fi
fi

# Parse parameters
while getopts "k:m:a:" option
do
case "${option}"
in
k) RELEEM_API_KEY=${OPTARG};;
m) MYSQL_MEMORY_LIMIT=${OPTARG};;
a) RELEEM_APPLY_CONFIG=1;;
esac
done

echo -e "\033[34m\n* Checking the environment...\033[0m"

# Check RELEEM_API_KEY is not empty
if [ -z "$RELEEM_API_KEY" ]; then
    echo >&2 "RELEEM_API_KEY is empty please sign up at https://releem.com/appsignup to get your Releem API key. Aborting."
    exit 1;
fi

command -v curl >/dev/null 2>&1 || { echo >&2 "Curl is not installed. Please install Curl. Aborting."; exit 1; }
command -v perl >/dev/null 2>&1 || { echo >&2 "Perl is not installed. Please install Perl. Aborting."; exit 1; }
perl -e "use JSON;" >/dev/null 2>&1 || { echo >&2 "Perl module JSON is not installed. Please install Perl module JSON. Aborting."; exit 1; }

# Check if the tmp folder exists
if [ -d "$MYSQLCONFIGURER_PATH" ]; then
    # Clear tmp directory
    rm $MYSQLCONFIGURER_PATH/*
else
    # Create tmp directory
    mkdir $MYSQLCONFIGURER_PATH
fi

# Check if MySQLTuner already downloaded and download if it doesn't exist
if [ ! -f "$MYSQLTUNER_FILENAME" ]; then
    # Download latest version of the MySQLTuner
    curl -s -o $MYSQLTUNER_FILENAME -L https://raw.githubusercontent.com/major/MySQLTuner-perl/v1.9.9/mysqltuner.pl
fi

echo -e "\033[34m\n* Collecting metrics...\033[0m"

# Collect MySQL metrics
if perl $MYSQLTUNER_FILENAME --json --verbose --notbstat --forcemem=$MYSQL_MEMORY_LIMIT --outputfile="$MYSQLTUNER_REPORT" --defaults-file ~/.my.cnf > /dev/null; then

    echo -e "\033[34m\n* Sending metrics to Releem Cloud Platform...\033[0m"

    # Send metrics to Releem Platform. The answer is the configuration file for MySQL
    curl -s -d @$MYSQLTUNER_REPORT -H "x-releem-api-key: $RELEEM_API_KEY" -H "Content-Type: application/json" -X POST https://api.releem.com/v1/mysql -o "$MYSQLCONFIGURER_CONFIGFILE"

    echo -e "\033[34m\n* Downloading recommended MySQL configuration from Releem Cloud Platform...\033[0m"

    # Show recommended configuration and exit
    msg="\n\n\n#---------------Releem Agent Report-------------\n\n"
    printf "${msg}"

    echo -e "1. Recommended MySQL configuration downloaded to /tmp/.mysqlconfigurer/z_aiops_mysql.cnf"
    echo
    echo -e "2. To check MySQL Performance Score please visit https://app.releem.com/dashboard?menu=metrics"
    echo
    if [ "$RELEEM_APPLY_CONFIG" = "1" ]; then
        releem_apply_config
    else
        echo -e "3. To apply the recommended configuration please read documentation https://app.releem.com/dashboard"
    fi

    exit
else

    # If error then show report and exit
    errormsg="    \
    \n\n\n\n--------Releem Agent completed with error--------\n   \
    \nCheck /tmp/.mysqlconfigurer/mysqltunerreport.json for details \n \
    \n--------Please fix the error and run Releem Agent again--------\n"
    printf "${errormsg}" >&2
    exit 1
fi
