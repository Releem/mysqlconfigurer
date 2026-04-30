#!/bin/bash
# Run all Releem agent Linux tests in sequence.
# Tests 1-4 are run sequentially: 1 must pass before 3, 3 before 4.
# Test 2 is run independently after cleanup.
#
# Usage:
#   ./run_all.sh [--test 1|2|3|4] [--skip-cleanup]
#
# Required env vars:
#   RELEEM_API_KEY, MYSQL_ROOT_PASSWORD, OS_VERSION
#
# Optional env vars:
#   RELEEM_EXISTING_USER_PASSWORD  - password for test 2 pre-existing user
#   INSTALL_SCRIPT                 - path to install.sh (default: /tmp/releem_tests/install.sh)
#   CONFIGURER_SCRIPT              - path to mysqlconfigurer.sh (default: /tmp/releem_tests/mysqlconfigurer.sh)

set -eo pipefail

: "${RELEEM_API_KEY:?RELEEM_API_KEY must be set}"
: "${MYSQL_ROOT_PASSWORD:?MYSQL_ROOT_PASSWORD must be set}"
: "${OS_VERSION:?OS_VERSION must be set}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ONLY_TEST=""
SKIP_CLEANUP=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --test)  ONLY_TEST="$2"; shift 2 ;;
        --skip-cleanup) SKIP_CLEANUP=true; shift ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

OVERALL_PASS=0
OVERALL_FAIL=0

run_test() {
    local num="$1" script="$2" name="$3"
    if [[ -n "$ONLY_TEST" && "$ONLY_TEST" != "$num" && "$ONLY_TEST" != "all" ]]; then
        return 0
    fi

    echo ""
    echo "######################################"
    echo "# Running $name"
    echo "######################################"

    if bash "$SCRIPT_DIR/$script"; then
        echo "[SUITE] $name: PASSED"
        OVERALL_PASS=$((OVERALL_PASS + 1))
    else
        echo "[SUITE] $name: FAILED"
        OVERALL_FAIL=$((OVERALL_FAIL + 1))
    fi
}

# Ensure scripts are in /tmp/releem_tests
mkdir -p /tmp/releem_tests
_copy_if_different() {
    local src="$1" dst="$2"
    if [[ -f "$src" ]] && [[ "$(realpath "$src" 2>/dev/null)" != "$(realpath "$dst" 2>/dev/null)" ]]; then
        cp "$src" "$dst"
    fi
}
_copy_if_different "${INSTALL_SCRIPT:-/tmp/install.sh}"           /tmp/releem_tests/install.sh
_copy_if_different "${CONFIGURER_SCRIPT:-/tmp/mysqlconfigurer.sh}" /tmp/releem_tests/mysqlconfigurer.sh
chmod +x /tmp/releem_tests/*.sh 2>/dev/null || true

export INSTALL_SCRIPT="/tmp/releem_tests/install.sh"
export CONFIGURER_SCRIPT="/tmp/releem_tests/mysqlconfigurer.sh"

if [[ -z "$ONLY_TEST" || "$ONLY_TEST" == "all" || "$ONLY_TEST" == "1" ]]; then
    run_test 1 "test_01_install_auto.sh" "Test 1: Fresh install (auto user creation)"
fi

if [[ -z "$ONLY_TEST" || "$ONLY_TEST" == "all" || "$ONLY_TEST" == "2" ]]; then
    run_test 2 "test_02_install_existing_user.sh" "Test 2: Install with pre-existing user"
    # Re-run test 1 to restore state for tests 3 and 4 (if running full suite)
    if [[ -z "$ONLY_TEST" || "$ONLY_TEST" == "all" ]]; then
        echo ""
        echo "[SUITE] Re-running Test 1 to restore state for Tests 3 and 4..."
        bash "$SCRIPT_DIR/test_01_install_auto.sh" || true
    fi
fi

if [[ -z "$ONLY_TEST" || "$ONLY_TEST" == "all" || "$ONLY_TEST" == "3" ]]; then
    run_test 3 "test_03_apply_config.sh" "Test 3: Apply configuration"
fi

if [[ -z "$ONLY_TEST" || "$ONLY_TEST" == "all" || "$ONLY_TEST" == "4" ]]; then
    run_test 4 "test_04_rollback_config.sh" "Test 4: Rollback configuration"
fi

echo ""
echo "========================================="
echo "OVERALL TEST SUITE RESULTS"
echo "  PASSED: $OVERALL_PASS"
echo "  FAILED: $OVERALL_FAIL"
echo "========================================="

if [[ $OVERALL_FAIL -gt 0 ]]; then
    exit 1
fi
exit 0
