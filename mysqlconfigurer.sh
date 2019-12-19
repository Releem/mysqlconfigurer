#!/bin/bash

# Variables
MYSQLCONFIGURER_PATH='/tmp/.mysqlconfigurer'
MYSQLTUNER_FILENAME=$MYSQLCONFIGURER_PATH'/mysqltuner.pl'
MYSQLTUNER_REPORT=$MYSQLCONFIGURER_PATH'/mysqltunerreport.json'
MYSQLCONFIGURER_CONFIGFILE=$MYSQLCONFIGURER_PATH'/z_aiops_mysql.conf'

command -v curl >/dev/null 2>&1 || { echo >&2 "Curl is not installed. Please install Curl. Aborting."; exit 1; }
command -v perl >/dev/null 2>&1 || { echo >&2 "Perl is not installed. Please install Perl. Aborting."; exit 1; }

# Check if the tmp folder exists
if [ -d "$DIR" ]; then
    # Clear tmp directory
    rm $MYSQLCONFIGURER_PATH/*
else
    # Create tmp directory 
    mkdir $MYSQLCONFIGURER_PATH
fi

# Download last version of the MySQLTuner
curl -o $MYSQLTUNER_FILENAME -L http://mysqltuner.pl/

# Run MySQLTuner for creating report in the JSON format
perl $MYSQLTUNER_FILENAME --json --verbose --notbstat --outputfile="$MYSQLTUNER_REPORT"

# Post MySQLTuner report in the AIOps service. The answer is the configuration file for MySQL
curl -d \"@$MYSQLTUNER_REPORT\" -X POST https://api.servers-support.com/v1/mysql -o "$MYSQLCONFIGURER_CONFIGFILE"