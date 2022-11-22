#!/bin/bash
# mysqlconfigurer.sh - Version 1.0.0
# (C) Releem, Inc 2022
# All rights reserved

# Variables
MYSQLCONFIGURER_PATH="/tmp/.mysqlconfigurer/"
RELEEM_CONF_FILE="/opt/releem/releem.conf"
MYSQLTUNER_FILENAME=$MYSQLCONFIGURER_PATH"mysqltuner.pl"
MYSQLTUNER_REPORT=$MYSQLCONFIGURER_PATH"mysqltunerreport.json"
MYSQLCONFIGURER_CONFIGFILE=$MYSQLCONFIGURER_PATH"z_aiops_mysql.cnf"
MYSQL_MEMORY_LIMIT=0
VERSION="1.0.0"
RELEEM_INSTALL_PATH=$MYSQLCONFIGURER_PATH"install.sh"

function update_agent() {
  NEW_VER=$(curl  -s -L https://releem.s3.amazonaws.com/current_version_agent)
  if [ "$VERSION" \< "$NEW_VER" ]
  then
      printf "\033[37m\n* Updating script \e[31;1m%s\e[0m -> \e[32;1m%s\e[0m\n" "$VERSION" "$NEW_VER"
      curl -s -L https://releem.s3.amazonaws.com/install.sh > "$RELEEM_INSTALL_PATH"
      RELEEM_API_KEY=$RELEEM_API_KEY exec bash "$RELEEM_INSTALL_PATH" -u
  fi

}
function wait_restart() {
  sleep 1
  flag=0
  spin[0]="-"
  spin[1]="\\"
  spin[2]="|"
  spin[3]="/"
#  echo -n "Waiting for restarted mysql ${spin[0]}"
  printf "\033[37m\n* Waiting for mysql service to start 120 seconds ${spin[0]}"

  while !(mysqladmin ping > /dev/null 2>&1)
  do
    flag=$(($flag + 1))
    if [ $flag == 120 ]; then
#        echo "$flag break"
        break
    fi
    i=`expr $flag % 4`
    #echo -ne "\b${spin[$i]}"
    printf "\b${spin[$i]}"
    sleep 1
  done
  printf "\033[0m\n"
}

function check_mysql_version() {

    if [ ! -f $MYSQLTUNER_REPORT ]; then
        printf "\033[37m\n* Please try again later or run Releem Agent manually:\033[0m"
        printf "\033[32m\n bash /opt/releem/mysqlconfigurer.sh \033[0m\n\n"
        exit 1;
    fi
    mysql_version=$(grep -o '"Version":"[^"]*' $MYSQLTUNER_REPORT  | grep -o '[^"]*$')
    if [ -z $mysql_version ]; then
        printf "\033[37m\n* Please try again later or run Releem Agent manually:\033[0m"
        printf "\033[32m\n bash /opt/releem/mysqlconfigurer.sh \033[0m\n\n"
        exit 1;
    fi
    requiredver="5.6.8"
    if [ "$(printf '%s\n' "$mysql_version" "$requiredver" | sort -V | head -n1)" = "$requiredver" ]; then
        return 0
    else
        return 1
    fi
}


function releem_rollback_config() {
    printf "\033[31m\n* Rolling back MySQL configuration!\033[0m\n"
    if ! check_mysql_version; then
        printf "\033[31m\n* MySQL version is lower than 5.6.7. Check the documentation https://github.com/Releem/mysqlconfigurer#how-to-apply-the-recommended-configuration for applying the configuration. \033[0m\n"
        exit 1
    fi
    if [ -z "$RELEEM_MYSQL_CONFIG_DIR" ]; then
        printf "\033[37m\n* MySQL configuration directory is not found.\033[0m"
        printf "\033[37m\n* Try to reinstall Releem Agent, and please set the my.cnf location.\033[0m"
        exit 1;
    fi
    if [ -z "$RELEEM_MYSQL_RESTART_SERVICE" ]; then
        printf "\033[37m\n* The command to restart the MySQL service was not found. Try to reinstall Releem Agent.\033[0m"
        exit 1;
    fi

    FLAG_RESTART_SERVICE=1
    if [ -z "$RELEEM_RESTART_SERVICE" ]; then
    	read -p "Please confirm restart MySQL service? (Y/N) " -n 1 -r
      echo    # move to a new line
      if [[ ! $REPLY =~ ^[Yy]$ ]]
      then
        printf "\033[37m\n* A confirmation to restart the service has not been received. Releem recommended configuration has not been roll back.\033[0m\n"
        FLAG_RESTART_SERVICE=0
      fi
    elif [ "$RELEEM_RESTART_SERVICE" -eq 0 ]; then
      FLAG_RESTART_SERVICE=0
    fi
    if [ "$FLAG_RESTART_SERVICE" -eq 0 ]; then
        exit 1
    fi

    printf "\033[31m\n* Deleting a configuration file... \033[0m\n"
    rm -rf $RELEEM_MYSQL_CONFIG_DIR/*
    #echo "----Test config-------"

    printf "\033[31m\n* Restarting with command '$RELEEM_MYSQL_RESTART_SERVICE'...\033[0m\n"
    eval "$RELEEM_MYSQL_RESTART_SERVICE" &
    wait_restart
    if [[ $(mysqladmin ping 2>/dev/null) == "mysqld is alive" ]];
    then
        printf "\033[32m\n* MySQL service started successfully!\033[0m\n"
    else
        printf "\033[31m\n* MySQL service failed to start in 120 seconds! Check mysql error log! \033[0m\n"
    fi
    exit 0
}

function releem_apply_config() {
    printf "\033[37m\n* Applying recommended MySQL configuration...\033[0m\n"
    if [ ! -f $MYSQLCONFIGURER_CONFIGFILE ]; then
        printf "\033[37m\n* Recommended MySQL configuration is not found.\033[0m"
        printf "\033[37m\n* Please apply recommended configuration later or run Releem Agent manually:\033[0m"
        printf "\033[32m\n bash /opt/releem/mysqlconfigurer.sh \033[0m\n\n"
        exit 1;
    fi
    if ! check_mysql_version; then
        printf "\033[31m\n* MySQL version is lower than 5.6.7. Check the documentation https://github.com/Releem/mysqlconfigurer#how-to-apply-the-recommended-configuration for applying the configuration. \033[0m\n"
        exit 1
    fi
    if [ -z "$RELEEM_MYSQL_CONFIG_DIR" ]; then
        printf "\033[37m\n* MySQL configuration directory is not found.\033[0m"
        printf "\033[37m\n* Try to reinstall Releem Agent, and please set the my.cnf location.\033[0m"
        exit 1;
    fi
    if [ -z "$RELEEM_MYSQL_RESTART_SERVICE" ]; then
        printf "\033[37m\n* The command to restart the MySQL service was not found. Try to reinstall Releem Agent.\033[0m"
        exit 1;
    fi
    printf "\033[37m\n* Copy file $MYSQLCONFIGURER_CONFIGFILE to directory $RELEEM_MYSQL_CONFIG_DIR/...\033[0m\n"
    yes | cp -fr $MYSQLCONFIGURER_CONFIGFILE $RELEEM_MYSQL_CONFIG_DIR/


    FLAG_RESTART_SERVICE=1
    if [ -z "$RELEEM_RESTART_SERVICE" ]; then
      read -p "Please confirm restart MySQL service? (Y/N) " -n 1 -r
      echo    # move to a new line
      if [[ ! $REPLY =~ ^[Yy]$ ]]
      then
          printf "\033[37m\n* A confirmation to restart the service has not been received. Releem recommended configuration has not been applied.\033[0m\n"
          FLAG_RESTART_SERVICE=0
      fi
    elif [ "$RELEEM_RESTART_SERVICE" -eq 0 ]; then
        FLAG_RESTART_SERVICE=0
    fi
    if [ "$FLAG_RESTART_SERVICE" -eq 0 ]; then
        exit 1
    fi

    #echo "-------Test config-------"
    printf "\033[37m\n* Restarting with command '$RELEEM_MYSQL_RESTART_SERVICE'...\033[0m\n"
    eval "$RELEEM_MYSQL_RESTART_SERVICE" &
    wait_restart

    if [[ $(mysqladmin ping 2>/dev/null) == "mysqld is alive" ]];
    then
        printf "\033[32m\n MySQL service started successfully!\033[0m\n"
        printf "\033[32m\n Recommended configuration applied successfully!\033[0m\n"
        printf "\n MySQL Performance Score and recommended configuration in Releem Customer Portal will update after 12 hours.\n"

    else
        printf "\033[31m\n MySQL service failed to start in 120 seconds! Check the MySQL error log! \033[0m\n"
        printf "\033[31m\n Try to roll back the configuration application using the command: \033[0m\n"
        printf "\033[32m\n bash /opt/releem/mysqlconfigurer.sh -r\033[0m\n\n"
    fi
    exit 0
}


function releem_runnig_cron() {
  HOUR=$(date +%I)
  MINUTE=$(date +%M)
  send_metrics
  if [ "${HOUR}" == "12" ] && [ "${MINUTE}" == "10" ];
  then
    get_config
    update_agent
  fi
  exit 0
}

function send_metrics() {
  #echo -e "\033[37m\n* Checking the environment...\033[0m"
  check_env
  ##### PARAMETERS #####
  CACHE_TTL="55"
  CACHE_FILE_STATUS="/tmp/releem.mysql.status.`echo $MYSQLCONFIGURER_CONFIGFILE | md5sum | cut -d" " -f1`.cache"
  CACHE_FILE_VARIABLES="/tmp/releem.mysql.variables.`echo $MYSQLCONFIGURER_CONFIGFILE | md5sum | cut -d" " -f1`.cache"
  EXEC_TIMEOUT="1"
  NOW_TIME=`date '+%s'`
  ##### RUN #####
  # Collect MySQL metrics
  #echo -e "\033[37m\n* Collecting metrics...\033[0m"

  if [ -s "${CACHE_FILE_STATUS}" ]; then
    CACHE_TIME=`stat -c"%Y" "${CACHE_FILE_STATUS}"`
  else
    CACHE_TIME=0
  fi
  DELTA_TIME=$((${NOW_TIME} - ${CACHE_TIME}))
  #
  if [ ${DELTA_TIME} -lt ${EXEC_TIMEOUT} ]; then
    sleep $((${EXEC_TIMEOUT} - ${DELTA_TIME}))
  elif [ ${DELTA_TIME} -gt ${CACHE_TTL} ]; then
    echo "" >> "${CACHE_FILE_STATUS}" # !!!
    DATACACHE=`mysql -sNe "show global status;"`
    echo "${DATACACHE}" > "${CACHE_FILE_STATUS}" # !!!
    chmod 640 "${CACHE_FILE_STATUS}"
  fi

  if [ -s "${CACHE_FILE_VARIABLES}" ]; then
    CACHE_TIME=`stat -c"%Y" "${CACHE_FILE_VARIABLES}"`
  else
    CACHE_TIME=0
  fi
  DELTA_TIME=$((${NOW_TIME} - ${CACHE_TIME}))
  #
  if [ ${DELTA_TIME} -lt ${EXEC_TIMEOUT} ]; then
    sleep $((${EXEC_TIMEOUT} - ${DELTA_TIME}))
  elif [ ${DELTA_TIME} -gt ${CACHE_TTL} ]; then
    echo "" >> "${CACHE_FILE_VARIABLES}" # !!!
    DATACACHE=`mysql -sNe "show global variables;"`
    echo "${DATACACHE}" > "${CACHE_FILE_VARIABLES}" # !!!
    chmod 640 "${CACHE_FILE_VARIABLES}"
  fi

  QUESTIONS=`cat ${CACHE_FILE_STATUS} | grep -w 'Questions' | awk '{print $2}'`
  TIMESTAMP=`stat -c"%Y" "${CACHE_FILE_STATUS}"`
  HOSTNAME=`cat ${CACHE_FILE_VARIABLES} | grep -w 'hostname' | awk '{print $2}'`

  JSON_STRING='{"Hostname": "'${HOSTNAME}'", "Timestamp":"'${TIMESTAMP}'", "ReleemMetrics": {"Questions": "'${QUESTIONS}'"}}'
  #echo -e "\033[37m\n* Sending metrics to Releem Cloud Platform...\033[0m"
  # Send metrics to Releem Platform. The answer is the configuration file for MySQL
  curl -s -d "$JSON_STRING" -H "x-releem-api-key: $RELEEM_API_KEY" -H "Content-Type: application/json" -X POST https://api.releem.com/v1/mysql
}

function check_env() {
  # Check RELEEM_API_KEY is not empty
  if [ -z "$RELEEM_API_KEY" ]; then
      echo >&2 "RELEEM_API_KEY is empty please sign up at https://releem.com/appsignup to get your Releem API key. Aborting."
      exit 1;
  fi
  command -v curl >/dev/null 2>&1 || { echo >&2 "Curl is not installed. Please install Curl. Aborting."; exit 1; }

}

function get_config() {
  echo -e "\033[37m\n* Checking the environment...\033[0m"
  check_env

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
      curl -s -o $MYSQLTUNER_FILENAME -L https://raw.githubusercontent.com/major/MySQLTuner-perl/fdd42e76857532002b8037cafddec3e38983dde8/mysqltuner.pl
      chmod +x $MYSQLTUNER_FILENAME
  fi

  echo -e "\033[37m\n* Collecting metrics to recommend a config...\033[0m"

  # Collect MySQL metrics
  if perl $MYSQLTUNER_FILENAME --json --verbose --notbstat --nocolstat --noidxstat --nopfstat --forcemem=$MYSQL_MEMORY_LIMIT --outputfile="$MYSQLTUNER_REPORT" --defaults-file ~/.my.cnf > /dev/null; then

      echo -e "\033[37m\n* Sending metrics to Releem Cloud Platform...\033[0m"

      # Send metrics to Releem Platform. The answer is the configuration file for MySQL
      curl -s -d @$MYSQLTUNER_REPORT -H "x-releem-api-key: $RELEEM_API_KEY" -H "Content-Type: application/json" -X POST https://api.releem.com/v1/mysql -o "$MYSQLCONFIGURER_CONFIGFILE"

      echo -e "\033[37m\n* Downloading recommended MySQL configuration from Releem Cloud Platform...\033[0m"

      # Show recommended configuration and exit
      msg="\n\n\n#---------------Releem Agent Report-------------\n\n"
      printf "${msg}"

      echo -e "1. Recommended MySQL configuration downloaded to /tmp/.mysqlconfigurer/z_aiops_mysql.cnf"
      echo
      echo -e "2. To check MySQL Performance Score please visit https://app.releem.com/dashboard?menu=metrics"
      echo
      echo -e "3. To apply the recommended configuration please read documentation https://app.releem.com/dashboard"
  else
      # If error then show report and exit
      errormsg="    \
      \n\n\n\n--------Releem Agent completed with error--------\n   \
      \nCheck /tmp/.mysqlconfigurer/mysqltunerreport.json for details \n \
      \n--------Please fix the error and run Releem Agent again--------\n"
      printf "${errormsg}" >&2
  fi

}

if test -f $RELEEM_CONF_FILE ; then
    . $RELEEM_CONF_FILE

    if [ ! -z $apikey ]; then
        RELEEM_API_KEY=$apikey
    fi
    if [ ! -z $memory_limit ]; then
        MYSQL_MEMORY_LIMIT=$memory_limit
    fi
    if [ ! -z $mysql_cnf_dir ]; then
        RELEEM_MYSQL_CONFIG_DIR=$mysql_cnf_dir
    fi
    if [ ! -z "$mysql_restart_service" ]; then
        RELEEM_MYSQL_RESTART_SERVICE=$mysql_restart_service
    fi
fi

# Parse parameters
while getopts "k:m:arc" option
do
case "${option}"
in
k) RELEEM_API_KEY=${OPTARG};;
m) MYSQL_MEMORY_LIMIT=${OPTARG};;
a) releem_apply_config;;
r) releem_rollback_config;;
c) releem_runnig_cron;;
esac
done

get_config
update_agent
