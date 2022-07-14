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

if test -f $RELEEM_CONF_FILE ; then
    . $RELEEM_CONF_FILE

    RELEEM_API_KEY=$apikey
    if [ ! -z $memory_limit ]; then
        MYSQL_MEMORY_LIMIT=$memory_limit
    fi
fi

# Parse parameters
while getopts "k:m:" option
do
case "${option}"
in
k) RELEEM_API_KEY=${OPTARG};;
m) MYSQL_MEMORY_LIMIT=${OPTARG};;
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
    echo -e "3. To apply the recommended configuration please read documentation https://app.releem.com/dashboard"
    
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
