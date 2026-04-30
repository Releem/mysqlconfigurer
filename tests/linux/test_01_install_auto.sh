#!/bin/bash
# Test 1: Fresh Releem Agent installation with automatic releem MySQL user creation.
# Pre-conditions:
#   - MySQL/MariaDB is running with world DB loaded
#   - releem MySQL user does NOT exist
#   - No previous Releem installation
# Required env vars:
#   RELEEM_API_KEY, MYSQL_ROOT_PASSWORD, OS_VERSION

set -eo pipefail
source "$(dirname "$0")/helpers.sh"

: "${RELEEM_API_KEY:?RELEEM_API_KEY must be set}"
: "${MYSQL_ROOT_PASSWORD:?MYSQL_ROOT_PASSWORD must be set}"
: "${OS_VERSION:?OS_VERSION must be set}"

HOSTNAME="releem-agent-test-${OS_VERSION}"

log_info "=== Test 1: Fresh installation with automatic releem user creation ==="
log_info "Hostname: $HOSTNAME"
log_info "OS_VERSION: $OS_VERSION"

# --- Pre-test cleanup ---
cleanup_releem
drop_releem_mysql_user

# --- Verify pre-conditions ---
DB_SERVICE=$(get_db_service)
assert_service_running "Pre: DB service running" "$DB_SERVICE"

result=$(mysql -u root -p"${MYSQL_ROOT_PASSWORD}" -sNe \
    "SELECT COUNT(*) FROM mysql.user WHERE User='releem';" 2>/dev/null)
if [[ "$result" -gt 0 ]]; then
    log_fail "Pre: releem MySQL user should not exist before test"
    exit 1
fi
log_info "Pre-conditions OK: DB running, no releem user"

# --- Run install.sh ---
log_info "Running install.sh..."
env \
    RELEEM_API_KEY="$RELEEM_API_KEY" \
    RELEEM_HOSTNAME="$HOSTNAME" \
    RELEEM_MYSQL_ROOT_PASSWORD="$MYSQL_ROOT_PASSWORD" \
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

assert_file_contains "releem.conf has hostname"     "/opt/releem/releem.conf" "$HOSTNAME"
assert_file_contains "releem.conf has api key"      "/opt/releem/releem.conf" "$RELEEM_API_KEY"

assert_mysql_user_exists "releem MySQL user created" "releem"

# Verify cron job added
if crontab -l 2>/dev/null | grep -q releem || ls /etc/cron.d/ 2>/dev/null | grep -q releem; then
    log_pass "Cron job for releem configured"
else
    log_fail "Cron job for releem NOT found"
fi

print_summary "Test 1: Fresh install with auto user creation"
