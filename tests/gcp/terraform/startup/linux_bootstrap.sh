#!/bin/bash
# Bootstrap script for GCP Linux test VMs.
# Installs MySQL/MariaDB, loads world sample DB, runs Releem tests from metadata payload.
set -euo pipefail

HOSTNAME_VALUE="${hostname}"
OS_VERSION_VALUE="${os_version}"
DB_VERSION="${db_version}"
DB_ROOT_PASSWORD="${db_root_password}"
RELEEM_API_KEY_VALUE="${releem_api_key}"
TEST_SELECTION_VALUE="${test_selection}"
TEST_PAYLOAD_B64="${test_payload_b64}"

LOG=/var/log/releem-bootstrap.log
exec > >(tee -a "$LOG") 2>&1

on_err() {
    rc=$?
    echo "[$(date)] ERROR: bootstrap failed with rc=$rc"
    echo "RELEEM_TEST_RESULT:FAIL"
    exit "$rc"
}
trap on_err ERR

echo "[$(date)] Bootstrap started: OS=$(. /etc/os-release && echo "$ID $VERSION_ID"), DB=$DB_VERSION"

systemctl stop unattended-upgrades 2>/dev/null || true
systemctl disable unattended-upgrades 2>/dev/null || true
while fuser /var/lib/dpkg/lock-frontend /var/cache/apt/archives/lock /var/lib/apt/lists/lock 2>/dev/null; do
    echo "[$(date)] Waiting for apt locks to release..."
    sleep 5
done

hostnamectl set-hostname "$HOSTNAME_VALUE" || hostname "$HOSTNAME_VALUE"
echo "127.0.1.1 $HOSTNAME_VALUE" >> /etc/hosts

if command -v apt-get &>/dev/null; then
    PKG_MGR="apt"
elif command -v dnf &>/dev/null; then
    PKG_MGR="dnf"
elif command -v yum &>/dev/null; then
    PKG_MGR="yum"
else
    echo "ERROR: No supported package manager found"
    exit 1
fi

echo "[$(date)] Installing prerequisites..."
export DEBIAN_FRONTEND=noninteractive
case "$PKG_MGR" in
    apt)
        apt-get update -qq
        apt-get install -y wget curl unzip gnupg lsb-release
        ;;
    yum|dnf)
        $PKG_MGR install -y wget curl unzip
        ;;
esac

install_mysql_apt() {
    case "$DB_VERSION" in
        mysql-8.0)
            apt-get install -y mysql-server
            ;;
        mysql-8.4)
            wget -q https://dev.mysql.com/get/mysql-apt-config_0.8.30-1_all.deb -O /tmp/mysql-apt-config.deb
            echo "mysql-apt-config mysql-apt-config/select-server select mysql-8.4-lts" | debconf-set-selections
            DEBIAN_FRONTEND=noninteractive dpkg -i /tmp/mysql-apt-config.deb
            apt-get update -qq
            apt-get install -y mysql-server
            ;;
        mariadb-10)
            apt-get install -y mariadb-server
            ;;
    esac
}

install_mysql_yum() {
    case "$DB_VERSION" in
        mysql-8.0)
            rpm --import https://repo.mysql.com/RPM-GPG-KEY-mysql-2023 2>/dev/null || true
            $PKG_MGR install -y https://dev.mysql.com/get/mysql80-community-release-el7-11.noarch.rpm 2>/dev/null || true
            $PKG_MGR install -y mysql-community-server
            ;;
        mysql-8.4)
            rpm --import https://repo.mysql.com/RPM-GPG-KEY-mysql-2023 2>/dev/null || true
            $PKG_MGR install -y https://dev.mysql.com/get/mysql84-community-release-el7-1.noarch.rpm 2>/dev/null || true
            $PKG_MGR install -y mysql-community-server
            ;;
        mariadb-10)
            cat > /etc/yum.repos.d/mariadb.repo <<'REPOEOF'
[mariadb]
name = MariaDB
baseurl = https://downloads.mariadb.com/MariaDB/mariadb-10.11/yum/rhel/$releasever/$basearch
gpgkey = https://downloads.mariadb.com/MariaDB/RPM-GPG-KEY-MariaDB
gpgcheck = 1
REPOEOF
            $PKG_MGR install -y MariaDB-server MariaDB-client
            ;;
    esac
}

echo "[$(date)] Installing $DB_VERSION..."
case "$PKG_MGR" in
    apt) install_mysql_apt ;;
    yum|dnf) install_mysql_yum ;;
esac

DB_SERVICE=mysql
for svc in mysql mysqld mariadb; do
    if systemctl list-unit-files | grep -q "^$svc\\.service"; then
        DB_SERVICE="$svc"
        break
    fi
done

echo "[$(date)] Starting $DB_SERVICE..."
systemctl enable "$DB_SERVICE" || true
systemctl start "$DB_SERVICE"
for i in $(seq 1 30); do
    if mysqladmin ping --silent 2>/dev/null; then
        break
    fi
    sleep 2
done

if mysql -u root -e "SELECT 1" &>/dev/null; then
    mysql -u root -e "ALTER USER 'root'@'localhost' IDENTIFIED WITH mysql_native_password BY '$DB_ROOT_PASSWORD'; FLUSH PRIVILEGES;"
elif mysql -u root --connect-expired-password -e "SELECT 1" &>/dev/null; then
    mysql -u root --connect-expired-password -e "ALTER USER 'root'@'localhost' IDENTIFIED WITH mysql_native_password BY '$DB_ROOT_PASSWORD'; FLUSH PRIVILEGES;"
fi

echo "[$(date)] Root password configured"

echo "[$(date)] Loading world sample database..."
wget -q https://downloads.mysql.com/docs/world-db.zip -O /tmp/world-db.zip
unzip -o /tmp/world-db.zip -d /tmp/
WORLD_SQL=$(find /tmp -name "world.sql" | head -1)
mysql -u root -p"$DB_ROOT_PASSWORD" < "$WORLD_SQL"

echo "[$(date)] Writing Linux test payload..."
mkdir -p /tmp/releem_tests
if [[ -z "$TEST_PAYLOAD_B64" ]]; then
    echo "ERROR: empty test payload"
    exit 1
fi
echo "$TEST_PAYLOAD_B64" | base64 -d > /tmp/releem-tests-payload.tar.gz
tar -xzf /tmp/releem-tests-payload.tar.gz -C /tmp/releem_tests
chmod +x /tmp/releem_tests/*.sh

echo "[$(date)] Running Linux test suite: $TEST_SELECTION_VALUE"
export RELEEM_API_KEY="$RELEEM_API_KEY_VALUE"
export MYSQL_ROOT_PASSWORD="$DB_ROOT_PASSWORD"
export OS_VERSION="$OS_VERSION_VALUE"
export INSTALL_SCRIPT="/tmp/releem_tests/install.sh"
export CONFIGURER_SCRIPT="/tmp/releem_tests/mysqlconfigurer.sh"

if bash /tmp/releem_tests/run_all.sh --test "$TEST_SELECTION_VALUE"; then
    echo "[$(date)] Linux tests passed"
    echo "RELEEM_TEST_RESULT:PASS"
else
    rc=$?
    echo "[$(date)] Linux tests failed rc=$rc"
    echo "RELEEM_TEST_RESULT:FAIL"
    exit "$rc"
fi

touch /tmp/bootstrap_complete
echo "[$(date)] Bootstrap complete"
echo "RELEEM_BOOTSTRAP_COMPLETE"
