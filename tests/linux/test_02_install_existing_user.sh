#!/bin/bash
# Test 2: Releem Agent installation when a releem MySQL user already exists.
# Pre-conditions:
#   - MySQL/MariaDB is running with world DB loaded
#   - releem MySQL user is pre-created manually (simulates ops team having set it up)
#   - No previous Releem installation (or cleaned up)
# Required env vars:
#   RELEEM_API_KEY, MYSQL_ROOT_PASSWORD, OS_VERSION
# Optional:
#   RELEEM_EXISTING_USER_PASSWORD  (generated if not set)

set -eo pipefail
source "$(dirname "$0")/helpers.sh"

: "${RELEEM_API_KEY:?RELEEM_API_KEY must be set}"
: "${MYSQL_ROOT_PASSWORD:?MYSQL_ROOT_PASSWORD must be set}"
: "${OS_VERSION:?OS_VERSION must be set}"

HOSTNAME="releem-agent-test-${OS_VERSION}"
RELEEM_USER_PASS="${RELEEM_EXISTING_USER_PASSWORD:-ReleemTest!$(date +%s)}"

log_info "=== Test 2: Installation with pre-existing releem MySQL user ==="
log_info "Hostname: $HOSTNAME"

# --- Pre-test cleanup ---
cleanup_releem
drop_releem_mysql_user

# --- Create releem MySQL user manually (simulate pre-existing user) ---
log_info "Creating releem MySQL user manually..."
mysql -u root -p"${MYSQL_ROOT_PASSWORD}" <<SQL
CREATE USER 'releem'@'127.0.0.1' IDENTIFIED BY '${RELEEM_USER_PASS}';
GRANT PROCESS, REPLICATION CLIENT, SHOW VIEW ON *.* TO 'releem'@'127.0.0.1';
GRANT SELECT ON mysql.* TO 'releem'@'127.0.0.1';
GRANT SELECT ON performance_schema.events_statements_summary_by_digest TO 'releem'@'127.0.0.1';
GRANT SELECT ON performance_schema.table_io_waits_summary_by_index_usage TO 'releem'@'127.0.0.1';
GRANT SELECT ON performance_schema.file_summary_by_instance TO 'releem'@'127.0.0.1';
FLUSH PRIVILEGES;
SQL

assert_mysql_user_exists "Pre: releem user pre-created" "releem"
assert_mysql_can_connect "Pre: releem user can connect" "releem" "$RELEEM_USER_PASS"

# --- Run install.sh with existing user credentials (no root password needed) ---
log_info "Running install.sh with pre-existing user credentials..."
env \
    RELEEM_API_KEY="$RELEEM_API_KEY" \
    RELEEM_HOSTNAME="$HOSTNAME" \
    RELEEM_MYSQL_LOGIN="releem" \
    RELEEM_MYSQL_PASSWORD="$RELEEM_USER_PASS" \
    RELEEM_CRON_ENABLE=1 \
    RELEEM_DB_MEMORY_LIMIT=0 \
    bash "$INSTALL_SCRIPT"

INSTALL_EXIT=$?
assert_zero "install.sh exited successfully" "$INSTALL_EXIT"

# --- Local assertions ---
assert_file_exists   "releem.conf created"           "/opt/releem/releem.conf"
assert_dir_exists    "releem conf dir created"        "/opt/releem/conf"
assert_file_exists   "releem-agent binary present"    "/opt/releem/releem-agent"
assert_service_running "releem-agent service running" "releem-agent"

assert_file_contains "releem.conf has hostname"    "/opt/releem/releem.conf" "$HOSTNAME"
assert_file_contains "releem.conf has api key"     "/opt/releem/releem.conf" "$RELEEM_API_KEY"
assert_file_contains "releem.conf has mysql login" "/opt/releem/releem.conf" "releem"

# Verify existing user password was NOT changed
assert_mysql_can_connect "Existing user credentials still work after install" "releem" "$RELEEM_USER_PASS"

print_summary "Test 2: Install with pre-existing releem user"
