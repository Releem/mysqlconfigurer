#!/usr/bin/env bash

load_mysqlconfigurer_functions() {
    local repo_root
    repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
    local configurer_script="${repo_root}/mysqlconfigurer.sh"

    RELEEM_TEST_MODE=1 source "${configurer_script}"
    # Prevent sourced script options from leaking into tests.
    set +e
}
