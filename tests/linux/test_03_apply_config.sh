#!/bin/bash
# Test 3: Apply recommended MySQL configuration via mysqlconfigurer.sh.
# Pre-conditions:
#   - Releem Agent is installed and running (run test_01 first)
#   - Agent has sent metrics and API has generated a recommendation
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

log_info "=== Test 3: Apply recommended MySQL configuration ==="
log_info "Hostname: $HOSTNAME"

# --- Verify pre-conditions ---
assert_service_running "Pre: releem-agent running"   "releem-agent"
assert_service_running "Pre: DB service running"     "$DB_SERVICE"
assert_file_exists     "Pre: releem.conf present"    "/opt/releem/releem.conf"

# --- Trigger agent to send metrics and fetch recommendation ---
log_info "Triggering agent to collect metrics and fetch recommendation..."
sudo /opt/releem/releem-agent -f 2>/dev/null || true

# Wait for the Releem API to generate a recommendation (up to 3 minutes)
log_info "Waiting up to 180s for API recommendation to be available..."
WAIT_SECONDS=0
WAIT_MAX=180
while [[ $WAIT_SECONDS -lt $WAIT_MAX ]]; do
    if [[ -f "$RELEEM_CONFIG_FILE" ]]; then
        log_info "Recommendation config file found after ${WAIT_SECONDS}s"
        break
    fi
    sleep 10
    WAIT_SECONDS=$((WAIT_SECONDS + 10))
    # Re-trigger agent in case first run didn't pull it
    if [[ $((WAIT_SECONDS % 60)) -eq 0 ]]; then
        sudo /opt/releem/releem-agent -f 2>/dev/null || true
    fi
done

# --- Apply configuration using mysqlconfigurer.sh -s automatic ---
log_info "Running mysqlconfigurer.sh -s automatic..."
env \
    RELEEM_API_KEY="$RELEEM_API_KEY" \
    RELEEM_RESTART_SERVICE=1 \
    bash "$CONFIGURER_SCRIPT" -s automatic

APPLY_EXIT=$?
assert_zero "mysqlconfigurer.sh -s automatic exited successfully" "$APPLY_EXIT"

# --- Local assertions ---
assert_file_exists "Recommended config file created" "$RELEEM_CONFIG_FILE"
assert_service_running "DB service still running after config apply" "$DB_SERVICE"
assert_service_running "releem-agent still running" "releem-agent"

# Verify the config file contains MySQL directives
assert_file_contains "Config file not empty (has [mysqld])" "$RELEEM_CONFIG_FILE" '\[mysqld\]'

# Verify MySQL is still accepting connections
assert_mysql_can_connect "MySQL still accepts connections after apply" "root" "$MYSQL_ROOT_PASSWORD" || true

print_summary "Test 3: Apply recommended MySQL configuration"
