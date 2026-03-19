#!/usr/bin/env bash

load_install_functions() {
    local repo_root
    repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
    local install_script="${repo_root}/install.sh"

    RELEEM_TEST_MODE=1 source "${install_script}"
    # Prevent sourced script options from leaking into tests.
    set +e
}
