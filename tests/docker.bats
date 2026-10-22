#!/usr/bin/env bats

setup() {
    TEST_TMPDIR="$(mktemp -d)"
    mkdir -p "$TEST_TMPDIR/releem" "$TEST_TMPDIR/mysql"
    sed -e "s|/opt/releem|$TEST_TMPDIR/releem|g" \
        -e "s|/etc/mysql/releem.conf.d|$TEST_TMPDIR/mysql|g" \
        "$BATS_TEST_DIRNAME/../docker/entrypoint.sh" > "$TEST_TMPDIR/entrypoint.sh"
    export AGENT_CALLS="$TEST_TMPDIR/agent-calls"
    cat > "$TEST_TMPDIR/releem/releem-agent" <<'AGENT'
#!/bin/bash
printf '%s\n' "$*" >> "$AGENT_CALLS"
exit "$DOWNLOAD_EXIT_CODE"
AGENT
    chmod +x "$TEST_TMPDIR/releem/releem-agent"
}

teardown() { rm -rf "$TEST_TMPDIR"; }

@test "Docker starts its command when initial configuration download fails" {
    run env DOWNLOAD_EXIT_CODE=1 bash "$TEST_TMPDIR/entrypoint.sh" \
        bash -c 'printf "daemon started\n"; exit 23'
    [ "$status" -eq 23 ]
    [[ "$output" == *"WARNING"* ]]
    [[ "$output" == *"daemon started"* ]]
    [ "$(cat "$AGENT_CALLS")" = '-f' ]
}

@test "Docker starts its command after successful initial configuration download" {
    run env DOWNLOAD_EXIT_CODE=0 bash "$TEST_TMPDIR/entrypoint.sh" \
        bash -c 'printf "daemon started\n"'
    [ "$status" -eq 0 ]
    [[ "$output" != *"WARNING"* ]]
    [[ "$output" == *"daemon started"* ]]
    [ "$(cat "$AGENT_CALLS")" = '-f' ]
}
