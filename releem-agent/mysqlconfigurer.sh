#!/bin/bash
# mysqlconfigurer.sh - Version 1.19.3
# (C) Releem, Inc 2022
# All rights reserved

# Variables
MYSQLCONFIGURER_PATH="/opt/releem/conf/"
RELEEM_CONF_FILE="/opt/releem/releem.conf"
MYSQLCONFIGURER_FILE_NAME="z_aiops_mysql.cnf"
MYSQLTUNER_FILENAME=$MYSQLCONFIGURER_PATH"mysqltuner.pl"
MYSQLTUNER_REPORT=$MYSQLCONFIGURER_PATH"mysqltunerreport.json"
RELEEM_MYSQL_VERSION=$MYSQLCONFIGURER_PATH"mysql_version"
MYSQLCONFIGURER_CONFIGFILE="${MYSQLCONFIGURER_PATH}${MYSQLCONFIGURER_FILE_NAME}"
MYSQL_MEMORY_LIMIT=0
VERSION="1.19.3"
RELEEM_INSTALL_PATH=$MYSQLCONFIGURER_PATH"install.sh"
logfile="releem-mysqlconfigurer.log"

# Set up a named pipe for logging
npipe=/tmp/$$.mysqlconfigurer.tmp
mknod $npipe p

# Log all output to a log for error checking
tee <$npipe $logfile &
exec 1>&-
exec 1>$npipe 2>&1

function on_exit() {
    curl -s -L -d @$logfile -H "x-releem-api-key: $RELEEM_API_KEY" -H "Content-Type: application/json" -X POST https://api.releem.com/v2/events/configurer_log
    rm -f $npipe
}

trap on_exit EXIT

function update_agent() {
    trap - EXIT
    /opt/releem/releem-agent start > /dev/null || true
    NEW_VER=$(curl  -s -L https://releem.s3.amazonaws.com/v2/current_version_agent)
    if [ "$NEW_VER" != "$VERSION" ]; then
        if [ "$(printf '%s\n' "$NEW_VER" "$VERSION" | sort -V | head -n1)" = "$VERSION" ]; 
        then
            printf "\033[37m\n * Updating script \e[31;1m%s\e[0m -> \e[32;1m%s\e[0m\n" "$VERSION" "$NEW_VER"
            curl -s -L https://releem.s3.amazonaws.com/v2/install.sh > "$RELEEM_INSTALL_PATH"
            RELEEM_API_KEY=$RELEEM_API_KEY exec bash "$RELEEM_INSTALL_PATH" -u
            /opt/releem/releem-agent --event=agent_updated > /dev/null
        fi
    fi
}

function non_blocking_wait() {
    PID=$1
    if [ ! -d "/proc/$PID" ]; then
        wait $PID
        CODE=$?
    else
        CODE=150
    fi
    return $CODE
}


function wait_restart() {
  sleep 1
  flag=0
  spin[0]="-"
  spin[1]="\\"
  spin[2]="|"
  spin[3]="/"
  printf "\033[37m\n Waiting for mysql service to start 1200 seconds ${spin[0]}"
  while /bin/true; do
    PID=$1
    non_blocking_wait $PID
    CODE=$?
    if [ $CODE -ne 150 ]; then
        printf "\033[0m\n PID $PID terminated with exit code $CODE"
        if [ $CODE -eq 0 ]; then
                RETURN_CODE=0
        else
                RETURN_CODE=7
        fi
        break
    fi
    flag=$(($flag + 1))
    if [ $flag == 1200 ]; then
        RETURN_CODE=6
        break
    fi
    i=`expr $flag % 4`
    printf "\b${spin[$i]}"
    sleep 1
  done
  printf "\033[0m\n"
  return $RETURN_CODE
}


function check_mysql_version() {

    if [ -f $MYSQLTUNER_REPORT ]; then
        mysql_version=$(grep -o '"Version":"[^"]*' $MYSQLTUNER_REPORT  | grep -o '[^"]*$')
    elif [ -f "$RELEEM_MYSQL_VERSION" ]; then
        mysql_version=$(cat $RELEEM_MYSQL_VERSION)
    else
        printf "\033[37m\n * Please try again later or run Releem Agent manually:\033[0m"
        printf "\033[32m\n  /opt/releem/releem-agent -f \033[0m\n\n"
        exit 1;
    fi
    if [ -z $mysql_version ]; then
        printf "\033[37m\n * Please try again later or run Releem Agent manually:\033[0m"
        printf "\033[32m\n /opt/releem/releem-agent -f \033[0m\n\n"
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
    printf "\033[31m\n * Rolling back MySQL configuration!\033[0m\n"
    if ! check_mysql_version; then
        printf "\033[31m\n * MySQL version is lower than 5.6.7. Check the documentation https://github.com/Releem/mysqlconfigurer#how-to-apply-the-recommended-configuration for applying the configuration. \033[0m\n"
        exit 2
    fi
    if [ -z "$RELEEM_MYSQL_CONFIG_DIR" -o ! -d "$RELEEM_MYSQL_CONFIG_DIR" ]; then
        printf "\033[37m\n * MySQL configuration directory is not found.\033[0m"
        printf "\033[37m\n * Try to reinstall Releem Agent, and please set the my.cnf location.\033[0m"
        exit 3;
    fi
    if [ -z "$RELEEM_MYSQL_RESTART_SERVICE" ]; then
        printf "\033[37m\n * The command to restart the MySQL service was not found. Try to reinstall Releem Agent.\033[0m"
        exit 4;
    fi

    FLAG_RESTART_SERVICE=1
    if [ -z "$RELEEM_RESTART_SERVICE" ]; then
    	read -p "Please confirm restart MySQL service? (Y/N) " -n 1 -r
      echo    # move to a new line
      if [[ ! $REPLY =~ ^[Yy]$ ]]
      then
        printf "\033[37m\n * A confirmation to restart the service has not been received. Releem recommended configuration has not been roll back.\033[0m\n"
        FLAG_RESTART_SERVICE=0
      fi
    elif [ "$RELEEM_RESTART_SERVICE" -eq 0 ]; then
      FLAG_RESTART_SERVICE=0
    fi
    if [ "$FLAG_RESTART_SERVICE" -eq 0 ]; then
        exit 5
    fi

    printf "\033[31m\n * Deleting a configuration file... \033[0m\n"
    rm -rf $RELEEM_MYSQL_CONFIG_DIR/$MYSQLCONFIGURER_FILE_NAME
    #echo "----Test config-------"
    if [ -f "${MYSQLCONFIGURER_PATH}${MYSQLCONFIGURER_FILE_NAME}.bkp" ]; then
        printf "\033[31m\n * Restoring a backup copy of the configuration file ${MYSQLCONFIGURER_PATH}${MYSQLCONFIGURER_FILE_NAME}.bkp... \033[0m\n"
        cp -f "${MYSQLCONFIGURER_PATH}${MYSQLCONFIGURER_FILE_NAME}.bkp" "${RELEEM_MYSQL_CONFIG_DIR}/${MYSQLCONFIGURER_FILE_NAME}"
    fi

    printf "\033[31m\n * Restarting with command '$RELEEM_MYSQL_RESTART_SERVICE'...\033[0m\n"
    eval "$RELEEM_MYSQL_RESTART_SERVICE" &
    wait_restart $!
    RESTART_CODE=$?

    #if [[ $(mysqladmin  ${connection_string}  --user=${MYSQL_LOGIN} --password=${MYSQL_PASSWORD} ping 2>/dev/null || true) == "mysqld is alive" ]];
    if [ $RESTART_CODE -eq 0 ];
    then
        printf "\n`date +%Y%m%d-%H:%M:%S`\033[32m The MySQL service restarted successfully!\033[0m\n"
        rm -f "${MYSQLCONFIGURER_PATH}${MYSQLCONFIGURER_FILE_NAME}.bkp"
    elif [ $RESTART_CODE -eq 6 ];
    then
        printf "\n`date +%Y%m%d-%H:%M:%S`\033[31m The MySQL service failed to restart in 1200 seconds! Check the MySQL error log! \033[0m\n"
    elif [ $RESTART_CODE -eq 7 ];
    then
        printf "\n`date +%Y%m%d-%H:%M:%S`\033[31m The MySQL service failed to restart with error! Check the MySQL error log! \033[0m\n" 
    fi
    /opt/releem/releem-agent --event=config_rollback > /dev/null
    exit "${RESTART_CODE}"
}



function releem_ps_mysql() {
    FLAG_CONFIGURE=1
    status_ps=$(mysql  ${connection_string}  --user=${MYSQL_LOGIN} --password=${MYSQL_PASSWORD} -BNe "show global variables like 'performance_schema'" 2>/dev/null | awk '{print $2}')
    if [ "$status_ps" != "ON" ]; then
        FLAG_CONFIGURE=0
    fi

    status_slowlog=$(mysql  ${connection_string}  --user=${MYSQL_LOGIN} --password=${MYSQL_PASSWORD} -BNe "show global variables like 'slow_query_log'" 2>/dev/null | awk '{print $2}')
    if [ "$status_slowlog" != "ON" ]; then
        FLAG_CONFIGURE=0
    fi

    if [ -n "$RELEEM_MYSQL_CONFIG_DIR" -a -d "$RELEEM_MYSQL_CONFIG_DIR" ]; then
        printf "\033[37m\n * Enabling Performance schema and SlowLog to collect metrics...\n\033[0m\n"
        echo -e "### This configuration was recommended by Releem. https://releem.com\n[mysqld]\nperformance_schema = 1\nslow_query_log = 1" > "$RELEEM_MYSQL_CONFIG_DIR/collect_metrics.cnf"
        chmod 644 $RELEEM_MYSQL_CONFIG_DIR/collect_metrics.cnf
    else
        printf "\033[31m\n MySQL configuration directory is not found.\033[0m"
        printf "\033[31m\n Try to reinstall Releem Agent.\033[0m"
        exit 3;
    fi
    if [ "$FLAG_CONFIGURE" -eq 1 ]; then
        printf "\033[37m\n * Performance schema and SlowLog are enabled for metrics collection.\033[0m\n"
        exit 0
    fi
    printf "\033[37m To apply changes to the mysql configuration, you need to restart the service\n\033[0m\n"
    FLAG_RESTART_SERVICE=1
    if [ -z "$RELEEM_RESTART_SERVICE" ]; then
        read -p "Please confirm restart MySQL service? (Y/N) " -n 1 -r
        echo    # move to a new line
        if [[ ! $REPLY =~ ^[Yy]$ ]]
        then
            printf "\033[31m A confirmation to restart the service has not been received. \033[0m\n"
            FLAG_RESTART_SERVICE=0
        fi
    elif [ "$RELEEM_RESTART_SERVICE" -eq 0 ]; then
        FLAG_RESTART_SERVICE=0
    fi
    if [ "$FLAG_RESTART_SERVICE" -eq 0 ]; then
        printf "\033[31m\n * For appling change in configuration mysql need restart service.\n\033[0m"
        printf "\033[31m Run the command \`bash /opt/releem/mysqlconfigurer.sh -p\` when it is possible to restart the service.\033[0m\n"
        exit 0
    fi
    #echo "-------Test config-------"
    printf "\033[37m Restarting service with command '$RELEEM_MYSQL_RESTART_SERVICE'...\033[0m\n"
    eval "$RELEEM_MYSQL_RESTART_SERVICE" &
    wait_restart $!
    RESTART_CODE=$?

    #if [[ $(mysqladmin  ${connection_string}  --user=${MYSQL_LOGIN} --password=${MYSQL_PASSWORD} ping 2>/dev/null || true) == "mysqld is alive" ]];
    if [ $RESTART_CODE -eq 0 ];
    then
        printf "\033[32m\n The MySQL service restarted successfully!\033[0m\n"
        printf "\033[32m\n Performance schema and Slow Log are enabled.\033[0m\n"
    elif [ $RESTART_CODE -eq 6 ];
    then
        printf "\033[31m\n The MySQL service failed to restart in 1200 seconds! Check the MySQL error log!\033[0m\n"
    elif [ $RESTART_CODE -eq 7 ];
    then
        printf "\n`date +%Y%m%d-%H:%M:%S`\033[31m The MySQL service failed to restart with error! Check the MySQL error log! \033[0m\n" 
    fi
    exit "${RESTART_CODE}"
}


function releem_apply_config() {
    if [ "$1" == "auto" ]; 
    then
        releem_apply_auto
    else
        releem_apply_manual
    fi
}
 
function releem_apply_auto() {
    /opt/releem/releem-agent --task=apply_config > /dev/null
    printf "\n`date +%Y%m%d-%H:%M:%S`\033[32m Request to create a task to apply the configuration was sended!\033[0m\n"
    exit 0
}

function releem_apply_manual() {
    printf "\n`date +%Y%m%d-%H:%M:%S`\033[37m Applying the recommended MySQL configuration...\033[0m\n"
    printf "\n`date +%Y%m%d-%H:%M:%S`\033[37m Getting the latest up-to-date configuration...\033[0m\n"
    /opt/releem/releem-agent -c >/dev/null 2>&1 || true

    if [ ! -f $MYSQLCONFIGURER_CONFIGFILE ]; then
        printf "\033[37m\n * Recommended MySQL configuration is not found.\033[0m"
        printf "\033[37m\n * Please apply recommended configuration later or run Releem Agent manually:\033[0m"
        printf "\033[32m\n /opt/releem/releem-agent -f \033[0m\n\n"
        exit 1;
    fi
    if ! check_mysql_version; then
        printf "\033[31m\n * MySQL version is lower than 5.6.7. Check the documentation https://github.com/Releem/mysqlconfigurer#how-to-apply-the-recommended-configuration for applying the configuration. \033[0m\n"
        exit 2
    fi
    if [ -z "$RELEEM_MYSQL_CONFIG_DIR" -o ! -d "$RELEEM_MYSQL_CONFIG_DIR" ]; then
        printf "\033[37m\n * MySQL configuration directory is not found.\033[0m"
        printf "\033[37m\n * Try to reinstall Releem Agent, and please set the my.cnf location.\033[0m"
        exit 3;
    fi
    if [ -z "$RELEEM_MYSQL_RESTART_SERVICE" ]; then
        printf "\033[37m\n * The command to restart the MySQL service was not found. Try to reinstall Releem Agent.\033[0m"
        exit 4;
    fi
    diff_cmd=$(which diff || true)
    if [ -n "$diff_cmd" ];then
        diff "${RELEEM_MYSQL_CONFIG_DIR}/${MYSQLCONFIGURER_FILE_NAME}" "$MYSQLCONFIGURER_CONFIGFILE" > /dev/null 2>&1
        retVal=$?
        if [ $retVal -eq 0 ];
        then
            printf "\n`date +%Y%m%d-%H:%M:%S`\033[32m The new configuration does not differ from the current one applied. No restart is required.!\033[0m\n"        
            exit 0
        fi
    fi

    FLAG_RESTART_SERVICE=1
    if [ -z "$RELEEM_RESTART_SERVICE" ]; then
      read -p "Please confirm the MySQL service restart? (Y/N) " -n 1 -r
      echo    # move to a new line
      if [[ ! $REPLY =~ ^[Yy]$ ]]
      then
          printf "\n`date +%Y%m%d-%H:%M:%S`\033[37m A confirmation to restart the service has not been received. Releem recommended configuration has not been applied.\033[0m\n"
          FLAG_RESTART_SERVICE=0
      fi
    elif [ "$RELEEM_RESTART_SERVICE" -eq 0 ]; then
        FLAG_RESTART_SERVICE=0
    fi
    if [ "$FLAG_RESTART_SERVICE" -eq 0 ]; then
        exit 5
    fi

    printf "\n`date +%Y%m%d-%H:%M:%S`\033[37m Copying file $MYSQLCONFIGURER_CONFIGFILE to directory $RELEEM_MYSQL_CONFIG_DIR/...\033[0m\n"
    if [ ! -f "${MYSQLCONFIGURER_PATH}${MYSQLCONFIGURER_FILE_NAME}.bkp" ]; then
        yes | cp -f "${RELEEM_MYSQL_CONFIG_DIR}/${MYSQLCONFIGURER_FILE_NAME}" "${MYSQLCONFIGURER_PATH}${MYSQLCONFIGURER_FILE_NAME}.bkp"
    fi    
    yes | cp -fr $MYSQLCONFIGURER_CONFIGFILE $RELEEM_MYSQL_CONFIG_DIR/
    chmod 644 $RELEEM_MYSQL_CONFIG_DIR/*

    #echo "-------Test config-------"
    printf "\n`date +%Y%m%d-%H:%M:%S`\033[37m Restarting MySQL with the command '$RELEEM_MYSQL_RESTART_SERVICE'...\033[0m\n"
    eval "$RELEEM_MYSQL_RESTART_SERVICE" &
    wait_restart $!
    RESTART_CODE=$?

    if [ $RESTART_CODE -eq 0 ];
    then
        printf "\n`date +%Y%m%d-%H:%M:%S`\033[32m The MySQL service restarted successfully!\033[0m\n"
        printf "\n`date +%Y%m%d-%H:%M:%S`\033[32m Recommended configuration applied successfully!\033[0m\n"
        printf "\n`date +%Y%m%d-%H:%M:%S` Releem Score and Unapplied recommendations in the Releem Dashboard will be updated in a few minutes.\n"
        rm -f "${MYSQLCONFIGURER_PATH}${MYSQLCONFIGURER_FILE_NAME}.bkp"
    elif [ $RESTART_CODE -eq 6 ];
    then
        printf "\n`date +%Y%m%d-%H:%M:%S`\033[31m MySQL service failed to restart in 1200 seconds! \033[0m\n"
        printf "\n`date +%Y%m%d-%H:%M:%S`\033[31m Wait for the MySQL service to start and Check the MySQL error log!\033[0m\n"

        printf "\n`date +%Y%m%d-%H:%M:%S`\033[31m Try to roll back the configuration application using the command: \033[0m\n"
        printf "\n`date +%Y%m%d-%H:%M:%S`\033[32m bash /opt/releem/mysqlconfigurer.sh -r\033[0m\n\n"
    elif [ $RESTART_CODE -eq 7 ];
    then
        printf "\n`date +%Y%m%d-%H:%M:%S`\033[31m MySQL service failed to restart! Check the MySQL error log! \033[0m\n"
        printf "\n`date +%Y%m%d-%H:%M:%S`\033[31m Try to roll back the configuration application using the command: \033[0m\n"
        printf "\n`date +%Y%m%d-%H:%M:%S`\033[32m bash /opt/releem/mysqlconfigurer.sh -r\033[0m\n\n"
    fi
    /opt/releem/releem-agent --event=config_applied > /dev/null
    printf "\n`date +%Y%m%d-%H:%M:%S`\033[32m Notification to Releem Platform was sent successfully!\033[0m\n"

    exit "${RESTART_CODE}"
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
  #echo -e "\033[37m\n * Checking the environment...\033[0m"
  check_env
  ##### PARAMETERS #####
  CACHE_TTL="55"
  CACHE_FILE_STATUS="/tmp/releem.mysql.status.`echo $MYSQLCONFIGURER_CONFIGFILE | md5sum | cut -d" " -f1`.cache"
  CACHE_FILE_VARIABLES="/tmp/releem.mysql.variables.`echo $MYSQLCONFIGURER_CONFIGFILE | md5sum | cut -d" " -f1`.cache"
  EXEC_TIMEOUT="1"
  NOW_TIME=`date '+%s'`
  ##### RUN #####
  # Collect MySQL metrics
  #echo -e "\033[37m\n * Collecting metrics...\033[0m"

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
  #echo -e "\033[37m\n * Sending metrics to Releem Cloud Platform...\033[0m"
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
  echo -e "\033[37m\n * Checking the environment...\033[0m"
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

  echo -e "\033[37m\n * Collecting metrics to recommend a config...\033[0m"

  # Collect MySQL metrics
  if perl $MYSQLTUNER_FILENAME --json --verbose --notbstat --nocolstat --noidxstat --nopfstat --forcemem=$MYSQL_MEMORY_LIMIT --outputfile="$MYSQLTUNER_REPORT" --user=${MYSQL_LOGIN} --pass=${MYSQL_PASSWORD}  ${connection_string}  > /dev/null; then

      echo -e "\033[37m\n * Sending metrics to Releem Cloud Platform...\033[0m"

      # Send metrics to Releem Platform. The answer is the configuration file for MySQL
      curl -s -d @$MYSQLTUNER_REPORT -H "x-releem-api-key: $RELEEM_API_KEY" -H "Content-Type: application/json" -X POST https://api.releem.com/v1/mysql -o "$MYSQLCONFIGURER_CONFIGFILE"

      echo -e "\033[37m\n * Downloading recommended MySQL configuration from Releem Cloud Platform...\033[0m"

      # Show recommended configuration and exit
      msg="\n\n#---------------Releem Agent Report-------------\n\n"
      printf "${msg}"

      echo -e "1. Recommended MySQL configuration downloaded to ${MYSQLCONFIGURER_CONFIGFILE}"
      echo
      echo -e "2. To check MySQL Performance Score please visit https://app.releem.com/dashboard?menu=metrics"
      echo
      echo -e "3. To apply the recommended configuration please read documentation https://app.releem.com/dashboard"
  else
      # If error then show report and exit
      errormsg="    \
      \n\n\n\n--------Releem Agent completed with error--------\n   \
      \nCheck $MYSQLTUNER_REPORT for details \n \
      \n--------Please fix the error and run Releem Agent again--------\n"
      printf "${errormsg}" >&2
  fi

}
connection_string=""
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
    if [ ! -z "$mysql_user" ]; then
        MYSQL_LOGIN=$mysql_user
    fi
    if [ ! -z "$mysql_password" ]; then
        MYSQL_PASSWORD=$mysql_password
    fi
    if [ ! -z "$mysql_host" ]; then
        if [ -S "$mysql_host" ]; then
            connection_string="${connection_string} --socket=$mysql_host"
        else
            connection_string="${connection_string} --host=$mysql_host"
        fi
    else
        connection_string="${connection_string} --host=127.0.0.1"
    fi
    if [ ! -z "$mysql_port" ]; then
        connection_string="${connection_string} --port=$mysql_port"
    else
        connection_string="${connection_string} --port=3306"        
    fi
fi


# Parse parameters
while getopts "k:m:s:arcpu" option
do
  case "${option}" in
    k) RELEEM_API_KEY=${OPTARG};;
    m) MYSQL_MEMORY_LIMIT=${OPTARG};;
    a) releem_apply_manual;;
    s) releem_apply_config ${OPTARG};;
    r) releem_rollback_config;;
    c) get_config;;
    p) releem_ps_mysql;;
    u) update_agent; exit 0;;
  esac
done

printf "\033[37m\n\033[0m"
printf "\033[37m * To run Releem Agent manually please use the following command:\033[0m\n"
printf "\033[32m /opt/releem/releem-agent -f\033[0m\n\n"