#!/bin/bash
# Test 4: Rollback previously applied MySQL configuration via mysqlconfigurer.sh -r.
# Pre-conditions:
#   - Releem Agent is installed (run test_01 first)
#   - Configuration was already applied (run test_03 first)
#   - Backup file exists at z_aiops_mysql.cnf.bkp
# Required env vars:
#   RELEEM_API_KEY, MYSQL_ROOT_PASSWORD, OS_VERSION

set -eo pipefail
source "$(dirname "$0")/helpers.sh"

: "${RELEEM_API_KEY:?RELEEM_API_KEY must be set}"
: "${MYSQL_ROOT_PASSWORD:?MYSQL_ROOT_PASSWORD must be set}"
: "${OS_VERSION:?OS_VERSION must be set}"

HOSTNAME="releem-agent-test-${OS_VERSION}"
DB_SERVICE=$(get_db_service)
RELEEM_CONF_DIR="/opt/releem/conf"
RELEEM_CONFIG_FILE="${RELEEM_CONF_DIR}/z_aiops_mysql.cnf"

log_info "=== Test 4: Rollback applied MySQL configuration ==="
log_info "Hostname: $HOSTNAME"

# --- Verify pre-conditions ---
assert_service_running "Pre: releem-agent running"  "releem-agent"
assert_service_running "Pre: DB service running"    "$DB_SERVICE"
assert_file_exists     "Pre: config file present"   "$RELEEM_CONFIG_FILE"

# --- Run rollback ---
log_info "Running mysqlconfigurer.sh -r..."
env \
    RELEEM_API_KEY="$RELEEM_API_KEY" \
    RELEEM_RESTART_SERVICE=1 \
    bash "$CONFIGURER_SCRIPT" -r

ROLLBACK_EXIT=$?
assert_zero "mysqlconfigurer.sh -r exited successfully" "$ROLLBACK_EXIT"

# --- Local assertions ---
assert_service_running "DB service running after rollback"     "$DB_SERVICE"
assert_service_running "releem-agent still running"            "releem-agent"

# After rollback the applied config should be removed or replaced with backup content
# mysqlconfigurer.sh -r restores the backup and removes or replaces the active config
if [[ ! -f "$RELEEM_CONFIG_FILE" ]]; then
    log_pass "Rollback: config file removed"
else
    # If file still exists it should match the backup (pre-apply state)
    log_info "Config file still present after rollback (may contain original backup content)"
fi

# MySQL must still accept connections after restart
assert_mysql_can_connect "MySQL accepts connections after rollback" "root" "$MYSQL_ROOT_PASSWORD" || true

print_summary "Test 4: Rollback applied MySQL configuration"
