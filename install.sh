#!/bin/bash
# install.sh - Version 0.9.0
# (C) Releem, Inc 2022
# All rights reserved

# Releem installation script: install and set up the Releem Agent on supported Linux distributions
# using the package manager.

set -e
install_script_version=0.9.0
logfile="releem-install.log"

WORKDIR="/opt/releem"
CONF="$WORKDIR/releem.conf"

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
    (crontab -l 2>/dev/null | grep -v "$WORKDIR/mysqlconfigurer.sh" || true; echo "$RELEEM_CRON") | crontab -
}

function releem_update() {
    printf "\033[34m\n* Downloading latest version of Releem Agent...\033[0m\n"
    curl -s -L -o $WORKDIR/mysqlconfigurer.sh https://releem.s3.amazonaws.com/mysqlconfigurer.sh

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
    MYSQL_CNF_DIR="/etc/my.cnf"

    if [ -x "/usr/bin/dnf" ]; then
        package_manager='dnf'
    else
        package_manager='yum'
    fi

    $sudo_cmd $package_manager -y install net-tools perl-JSON perl-Data-Dumper perl-Getopt-Long

elif [ "$OS" = "Debian" ]; then
    MYSQL_CNF="/etc/mysql/my.cnf"
    printf "\033[34m\n* Installing dependences...\n\033[0m\n"

    $sudo_cmd apt-get update
    $sudo_cmd apt-get install -y --force-yes curl net-tools libjson-perl

else
    printf "\033[31mYour OS or distribution are not supported by this install script.\033[0m\n"
    exit;
fi

# Create work directory
if [ ! -e $CONF ]; then
    $sudo_cmd mkdir $WORKDIR
    $sudo_cmd mkdir $WORKDIR/conf
fi

printf "\033[34m\n* Downloading Releem Agent...\033[0m\n"
curl -o $WORKDIR/mysqlconfigurer.sh https://releem.s3.amazonaws.com/mysqlconfigurer.sh

printf "\033[34m\n* Checking ~/.my.cnf...\033[0m\n"
if [ ! -e ~/.my.cnf ]; then
    printf "\033[34m\n* Please create ~/.my.cnf file with the following content:\033[0m\n"
    echo -e "\t\t[client]"
    echo -e "\t\tuser=root"
    echo -e "\t\tpassword=[your MySQL root password]"
    read -p "Are you ready to proceed? (Y/N) " -n 1 -r
    echo    # move to a new line
    if [[ $REPLY =~ ^[Nn]$ ]]
    then
        exit 1
    fi
fi

RELEEM_COMMAND="/bin/bash $WORKDIR/mysqlconfigurer.sh -k $apikey"

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
$sudo_cmd echo "apikey=$apikey" > $CONF
printf "\033[34m\n* Adding MySQL Configuration Directory $WORKDIR/conf to Releem Agent configuration: $CONF\n\033[0m\n"
$sudo_cmd echo "mysql_cnf_dir=$WORKDIR/conf" >> $CONF

if [ -n "$MYSQL_LIMIT" ]; then
    RELEEM_COMMAND="/bin/bash $WORKDIR/mysqlconfigurer.sh -k $apikey -m $MYSQL_LIMIT"

    printf "\033[34m\n* Adding Memory Limit to the Releem Agent configuration: $CONF\n\033[0m\n"
    $sudo_cmd echo "memory_limit=$MYSQL_LIMIT" >> $CONF
fi

# Secure the configuration file
$sudo_cmd chmod 640 $CONF

if [ -z "$RELEEM_AGENT_DISABLE" ]; then
    # First run of Releem Agent to check MySQL Performance Score
    printf "\033[34m\n* Executing Releem Agent for first time...\033[0m\n"
    $sudo_cmd $RELEEM_COMMAND
fi

RELEEM_CRON="10 */12 * * * PATH=/bin:/sbin:/usr/bin:/usr/sbin $RELEEM_COMMAND"

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
