#!/bin/bash

# Variables
MYSQLCONFIGURER_PATH="/tmp/.mysqlconfigurer/"
MYSQLTUNER_FILENAME=$MYSQLCONFIGURER_PATH"mysqltuner.pl"
MYSQLTUNER_REPORT=$MYSQLCONFIGURER_PATH"mysqltunerreport.json"
MYSQLCONFIGURER_CONFIGFILE=$MYSQLCONFIGURER_PATH"z_aiops_mysql.conf"

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

# Download last version of the MySQLTuner
curl -s -o $MYSQLTUNER_FILENAME -L http://mysqltuner.pl/ 

# Run MySQLTuner for creating report in the JSON format
if perl $MYSQLTUNER_FILENAME --json --verbose --notbstat --outputfile="$MYSQLTUNER_REPORT" > /dev/null; then 

    # Post MySQLTuner report in the AIOps service. The answer is the configuration file for MySQL
    curl -s -d @$MYSQLTUNER_REPORT -H "Content-Type: application/json" -X POST https://api.servers-support.com/v1/mysql -o "$MYSQLCONFIGURER_CONFIGFILE"
    exit
else

    # If error then show report and exit
    errormsg="    \
    \n\n\n\n--------MySQLTuner completed with error--------\n   \
    \nCheck /tmp/.mysqlconfigurer/mysqltunerreport.json for details \n \
    \n--------Please fix the error and run again--------\n"
    printf "${errormsg}" >&2
    exit 1
fi