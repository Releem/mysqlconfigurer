#!/bin/bash
# (C) Releem, Inc 2020
# All rights reserved

# Releem installation script: install and set up the mysqlconfigurer on supported Linux distributions
# using the package manager.

set -e
install_script_version=1.0.0
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

apikey=
if [ -n "$RELEEM_API_KEY" ]; then
    apikey=$RELEEM_API_KEY
fi

if [ ! "$apikey" ]; then
    printf "\033[31mReleem API key is not available in RELEEM_API_KEY environment variable. Please sigh up at https://releem.com\033[0m\n"
    exit 1;
fi

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

# Root user detection
if [ "$(echo "$UID")" = "0" ]; then
    sudo_cmd=''
else
    sudo_cmd='sudo'
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

# Set the configuration
  if [ ! -e $CONF ]; then
    $sudo_cmd mkdir $WORKDIR
  fi
  if [ "$apikey" ]; then
    printf "\033[34m\n* Adding your API key to the Agent configuration: $CONF\n\033[0m\n"
    $sudo_cmd echo "export apikey=$apikey" > $CONF
  fi

# Secure the configuration file
$sudo_cmd chmod 640 $CONF

printf "\033[31mDownloading Releem MySQLConfigurer...\033[0m\n"
curl -o $WORKDIR/mysqlconfigurer.sh https://releem.s3.amazonaws.com/mysqlconfigurer.sh

