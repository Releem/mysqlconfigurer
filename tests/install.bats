#!/usr/bin/env bats

load "helpers/load_install.sh"

setup() {
    REPO_ROOT="$(cd "${BATS_TEST_DIRNAME}/.." && pwd)"
    INSTALL_SH="${REPO_ROOT}/install.sh"
    TEST_TMPDIR="$(mktemp -d)"
    MOCK_BIN="${TEST_TMPDIR}/mock-bin"
    mkdir -p "${MOCK_BIN}"
}

teardown() {
    rm -rf "${TEST_TMPDIR}"
}

create_mock_cmd() {
    local cmd_name="$1"
    local body="$2"
    cat >"${MOCK_BIN}/${cmd_name}" <<EOF
#!/usr/bin/env bash
${body}
EOF
    chmod +x "${MOCK_BIN}/${cmd_name}"
}

prepare_common_install_mocks() {
    create_mock_cmd "arch" 'echo "x86_64"'
    create_mock_cmd "sudo" '"$@"'
    create_mock_cmd "lsb_release" 'echo "Description: Ubuntu 24.04"'
    create_mock_cmd "crontab" 'exit 0'
    create_mock_cmd "hostname" 'echo "test-host"'
    create_mock_cmd "timeout" 'shift; "$@"'
    create_mock_cmd "pgrep" 'echo "1234"'
    create_mock_cmd "systemctl" 'exit 0'
    create_mock_cmd "apt-get" 'exit 0'
    create_mock_cmd "yum" 'exit 0'
    create_mock_cmd "dnf" 'exit 0'
    create_mock_cmd "curl" '
out=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) out="$2"; shift 2 ;;
    *) shift ;;
  esac
done
if [ -n "$out" ]; then
  mkdir -p "$(dirname "$out")"
  if [[ "$out" == *"releem-agent"* ]]; then
    cp /bin/true "$out"
  elif [[ "$out" == *"mysqlconfigurer.sh"* || "$out" == *"releem.sh"* ]]; then
    cat >"$out" <<'"'"'INNER'"'"'
#!/usr/bin/env bash
exit 0
INNER
    chmod +x "$out"
  else
    : >"$out"
  fi
fi
echo "200"
'
}

assert_exact_sql_payloads() {
    local log_file="$1"
    shift
    local index payload
    local -a actual_payloads=()
    local -a expected_payloads=("$@")

    while IFS= read -r -d '' payload; do
        actual_payloads+=("${payload}")
    done < "${log_file}"

    if [ "${#actual_payloads[@]}" -ne "${#expected_payloads[@]}" ]; then
        printf 'executed SQL count=%d, expected=%d; payloads=%s\n' \
            "${#actual_payloads[@]}" "${#expected_payloads[@]}" "${actual_payloads[*]}"
        return 1
    fi

    for ((index = 0; index < ${#expected_payloads[@]}; index++)); do
        if [ "${actual_payloads[index]}" != "${expected_payloads[index]}" ]; then
            printf 'SQL payload %d=<%s>, expected <%s>\n' \
                "${index}" "${actual_payloads[index]}" "${expected_payloads[index]}"
            return 1
        fi
    done
}

@test "detect_database_type defaults to mysql" {
    load_install_functions
    unset RELEEM_PG_HOST RELEEM_PG_LOGIN RELEEM_PG_PASSWORD RELEEM_PG_ROOT_PASSWORD RELEEM_PG_ROOT_LOGIN RELEEM_PG_TYPE
    unset RELEEM_MYSQL_HOST RELEEM_MYSQL_LOGIN RELEEM_MYSQL_PASSWORD RELEEM_MYSQL_ROOT_PASSWORD RELEEM_MYSQL_ROOT_LOGIN RELEEM_MYSQL_TYPE

    run detect_database_type
    [ "$status" -eq 0 ]
    [ "$database_type" = "mysql" ]
}

@test "detect_database_type prefers postgresql if both groups set" {
    load_install_functions
    RELEEM_PG_HOST="127.0.0.1"
    RELEEM_MYSQL_HOST="127.0.0.1"

    run detect_database_type
    [ "$status" -eq 0 ]
    [ "$database_type" = "postgresql" ]
}

@test "detect_database_type uses postgresql root login as postgresql signal" {
    load_install_functions
    unset RELEEM_PG_HOST RELEEM_PG_LOGIN RELEEM_PG_PASSWORD RELEEM_PG_ROOT_PASSWORD RELEEM_PG_TYPE
    unset RELEEM_MYSQL_HOST RELEEM_MYSQL_LOGIN RELEEM_MYSQL_PASSWORD RELEEM_MYSQL_ROOT_PASSWORD RELEEM_MYSQL_ROOT_LOGIN RELEEM_MYSQL_TYPE
    RELEEM_PG_ROOT_LOGIN="pgadmin"

    run detect_database_type
    [ "$status" -eq 0 ]
    [ "$database_type" = "postgresql" ]
}

@test "detect_database_type uses postgresql type flag as postgresql signal" {
    run bash -c '
        RELEEM_TEST_MODE=1 source "$1"
        unset RELEEM_PG_HOST RELEEM_PG_LOGIN RELEEM_PG_PASSWORD RELEEM_PG_ROOT_PASSWORD RELEEM_PG_ROOT_LOGIN
        unset RELEEM_MYSQL_HOST RELEEM_MYSQL_LOGIN RELEEM_MYSQL_PASSWORD RELEEM_MYSQL_ROOT_PASSWORD RELEEM_MYSQL_ROOT_LOGIN RELEEM_MYSQL_TYPE
        RELEEM_PG_TYPE="1"
        detect_database_type >/dev/null
        printf "%s" "$database_type"
    ' _ "${INSTALL_SH}"

    [ "$status" -eq 0 ]
    [ "$output" = "postgresql" ]
}

@test "detect_database_type uses mysql root login as mysql signal" {
    load_install_functions
    unset RELEEM_PG_HOST RELEEM_PG_LOGIN RELEEM_PG_PASSWORD RELEEM_PG_ROOT_PASSWORD RELEEM_PG_ROOT_LOGIN RELEEM_PG_TYPE
    unset RELEEM_MYSQL_HOST RELEEM_MYSQL_LOGIN RELEEM_MYSQL_PASSWORD RELEEM_MYSQL_ROOT_PASSWORD RELEEM_MYSQL_TYPE
    RELEEM_MYSQL_ROOT_LOGIN="admin"

    run detect_database_type
    [ "$status" -eq 0 ]
    [ "$database_type" = "mysql" ]
}

@test "detect_database_type uses mysql type flag as mysql signal" {
    run bash -c '
        RELEEM_TEST_MODE=1 source "$1"
        unset RELEEM_PG_HOST RELEEM_PG_LOGIN RELEEM_PG_PASSWORD RELEEM_PG_ROOT_PASSWORD RELEEM_PG_ROOT_LOGIN RELEEM_PG_TYPE
        unset RELEEM_MYSQL_HOST RELEEM_MYSQL_LOGIN RELEEM_MYSQL_PASSWORD RELEEM_MYSQL_ROOT_PASSWORD RELEEM_MYSQL_ROOT_LOGIN
        RELEEM_MYSQL_TYPE="1"
        detect_database_type
    ' _ "${INSTALL_SH}"

    [ "$status" -eq 0 ]
    [[ "$output" == *"Detected MySQL configuration."* ]]
}

@test "setup_mysql_connection_string builds host and default port" {
    run bash -c '
        RELEEM_TEST_MODE=1 source "$1"
        unset RELEEM_MYSQL_PORT
        RELEEM_MYSQL_HOST="10.1.2.3"
        setup_mysql_connection_string >/dev/null
        printf "conn=%s\nroot=%s\nhost=%s\n" "$connection_string" "$root_connection_string" "$mysql_user_host"
    ' _ "${INSTALL_SH}"

    [ "$status" -eq 0 ]
    [[ "$output" == *"conn= --host=10.1.2.3 --port=3306"* ]]
    [[ "$output" == *"root= --host=10.1.2.3"* ]]
    [[ "$output" == *"host=%"* ]]
}

@test "setup_mysql_connection_string uses socket and localhost host marker" {
    load_install_functions
    local sock="${TEST_TMPDIR}/mysql.sock"
    : >"$sock"
    RELEEM_MYSQL_HOST="$sock"

    run setup_mysql_connection_string
    [ "$status" -eq 0 ]
    [[ "$connection_string" == *"--socket=${sock}"* ]]
    [ "$mysql_user_host" = "localhost" ]
}

@test "setup_postgresql_connection_string enables peer mode without root password" {
    load_install_functions
    unset RELEEM_PG_ROOT_PASSWORD
    RELEEM_PG_HOST="10.2.0.3"
    RELEEM_PG_PORT="5433"

    run setup_postgresql_connection_string
    [ "$status" -eq 0 ]
    [[ "$pg_connection_string" == *"-h 10.2.0.3"* ]]
    [[ "$pg_connection_string" == *"-p 5433"* ]]
    [ "$pg_root_peer_connection" = "sudo -u postgres " ]
}

@test "setup_postgresql_connection_string uses explicit empty root password when env is set" {
    run bash -c '
        RELEEM_TEST_MODE=1 source "$1"
        RELEEM_PG_ROOT_PASSWORD=""
        RELEEM_PG_HOST="10.2.0.3"
        RELEEM_PG_PORT="5433"
        setup_postgresql_connection_string >/dev/null
        printf "root=%s\npeer=%s\n" "$pg_root_connection_string" "$pg_root_peer_connection"
    ' _ "${INSTALL_SH}"

    [ "$status" -eq 0 ]
    [[ "$output" == *"root= -h 10.2.0.3 -p 5433 -d postgres"* ]]
    [[ "$output" == *"peer="* ]]
    [[ "$output" != *"peer=sudo -u postgres "* ]]
}

@test "detect_instance_type defaults to local and accepts override" {
    load_install_functions
    unset RELEEM_INSTANCE_TYPE
    detect_instance_type
    [ "$instance_type" = "local" ]

    RELEEM_INSTANCE_TYPE="aws/rds"
    detect_instance_type
    [ "$instance_type" = "aws/rds" ]
}

@test "configure_connection_parameters dispatches by database_type" {
    load_install_functions
    called_mysql=0
    called_pg=0
    detect_mysql_commands() { called_mysql=1; }
    setup_mysql_connection_string() { :; }
    detect_postgresql_commands() { called_pg=1; }
    setup_postgresql_connection_string() { :; }

    database_type="mysql"
    configure_connection_parameters
    [ "$called_mysql" -eq 1 ]
    [ "$called_pg" -eq 0 ]

    called_mysql=0
    called_pg=0
    database_type="postgresql"
    configure_connection_parameters
    [ "$called_mysql" -eq 0 ]
    [ "$called_pg" -eq 1 ]
}

@test "configure_releem_agent writes postgresql ssl mode as an HCL boolean" {
    load_install_functions
    local workdir="${TEST_TMPDIR}/workdir"
    local conf="${workdir}/releem.conf"
    mkdir -p "${workdir}"

    database_type="postgresql"
    RELEEM_WORKDIR="${workdir}"
    RELEEM_CONF_FILE="${conf}"
    RELEEM_PG_SSL_MODE="false"
    apikey="k1"
    sudo_cmd=""
    unset PG_LOGIN PG_PASSWORD RELEEM_PG_HOST RELEEM_PG_PORT pg_service_name_cmd PG_CONF_DIR

    run configure_releem_agent

    [ "$status" -eq 0 ]
    run grep '^pg_ssl_mode=' "${conf}"
    [ "$status" -eq 0 ]
    [ "$output" = 'pg_ssl_mode=false' ] || return 1
}

@test "configure_releem_agent escapes postgresql credentials as HCL strings" {
    load_install_functions
    local workdir="${TEST_TMPDIR}/workdir"
    local conf="${workdir}/releem.conf"
    mkdir -p "${workdir}"

    database_type="postgresql"
    RELEEM_WORKDIR="${workdir}"
    RELEEM_CONF_FILE="${conf}"
    PG_LOGIN='role"reader'
    PG_PASSWORD=$'line1\\line2"\nline3\tend'
    apikey="k1"
    sudo_cmd=""
    unset RELEEM_PG_HOST RELEEM_PG_PORT RELEEM_PG_SSL_MODE pg_service_name_cmd PG_CONF_DIR

    run configure_releem_agent

    [ "$status" -eq 0 ]
    run grep '^pg_user=' "${conf}"
    [ "$status" -eq 0 ]
    [ "$output" = 'pg_user="role\"reader"' ] || return 1
    run grep '^pg_password=' "${conf}"
    [ "$status" -eq 0 ]
    [ "$output" = 'pg_password="line1\\line2\"\nline3\tend"' ] || return 1
}

@test "quote_hcl_string escapes quotes backslashes and control characters" {
    run bash -c '
        load_install() { RELEEM_TEST_MODE=1 source "$1"; }
        load_install "$1"
        declare -F quote_hcl_string >/dev/null || exit 99
        input=$(printf '"'"'line1\\line2"\nline3\tend\r'"'"')
        quote_hcl_string "${input}"
    ' _ "${INSTALL_SH}"

    [ "$status" -eq 0 ]
    [ "$output" = '"line1\\line2\"\nline3\tend\r"' ]
}

@test "setting_up_database_instance dispatches only for local instances" {
    load_install_functions
    mysql_setup_called=0
    pg_setup_called=0
    setting_up_local_mysql_instance() { mysql_setup_called=1; }
    setting_up_local_postgresql_instance() { pg_setup_called=1; }

    instance_type="local"
    database_type="mysql"
    setting_up_database_instance
    [ "$mysql_setup_called" -eq 1 ]
    [ "$pg_setup_called" -eq 0 ]

    mysql_setup_called=0
    pg_setup_called=0
    instance_type="local"
    database_type="postgresql"
    setting_up_database_instance
    [ "$mysql_setup_called" -eq 0 ]
    [ "$pg_setup_called" -eq 1 ]

    mysql_setup_called=0
    pg_setup_called=0
    instance_type="aws/rds"
    database_type="mysql"
    setting_up_database_instance
    [ "$mysql_setup_called" -eq 0 ]
    [ "$pg_setup_called" -eq 0 ]
}

@test "detect_mysql_service picks mariadb restart command via systemctl" {
    load_install_functions
    sudo_cmd=""
    create_mock_cmd "systemctl" '
if [ "$1" = "status" ] && [ "$2" = "mariadb" ]; then
  exit 0
fi
exit 1
'
    PATH="${MOCK_BIN}:${PATH}"

    run detect_mysql_service
    [ "$status" -eq 0 ]
    [[ "$service_name_cmd" == *"systemctl restart mariadb"* ]]
}

@test "detect_postgresql_service picks default postgresql service" {
    load_install_functions
    sudo_cmd=""
    create_mock_cmd "systemctl" '
if [ "$1" = "status" ] && [ "$2" = "postgresql" ]; then
  exit 0
fi
exit 1
'
    PATH="${MOCK_BIN}:${PATH}"

    run detect_postgresql_service
    [ "$status" -eq 0 ]
    [[ "$pg_service_name_cmd" == *"systemctl restart postgresql"* ]]
}

@test "enable_collect_queries passes restart approval to postgresql configurer" {
    load_install_functions
    cat >"${TEST_TMPDIR}/capture_configurer_env.sh" <<'EOF'
#!/usr/bin/env bash
if [ "${RELEEM_RESTART_SERVICE:-}" != "1" ]; then
  exit 42
fi
echo "RELEEM_RESTART_SERVICE=${RELEEM_RESTART_SERVICE:-}" >"${CONFIGURER_ENV_LOG}"
echo "args=$*" >>"${CONFIGURER_ENV_LOG}"
EOF
    chmod +x "${TEST_TMPDIR}/capture_configurer_env.sh"

    run env -u RELEEM_RESTART_SERVICE \
        CONFIGURER_ENV_LOG="${TEST_TMPDIR}/configurer.env" \
        bash -c '
            RELEEM_TEST_MODE=1 source "$1"
            set +e
            instance_type="local"
            database_type="postgresql"
            FLAG_PG_STAT_STATEMENTS=1
            sudo_cmd=""
            RELEEM_COMMAND="bash $2"
            enable_collect_queries
        ' _ "${INSTALL_SH}" "${TEST_TMPDIR}/capture_configurer_env.sh"

    [ "$status" -eq 0 ]
    run grep -E "^RELEEM_RESTART_SERVICE=1$" "${TEST_TMPDIR}/configurer.env"
    [ "$status" -eq 0 ]
    run grep -E "^args=-p$" "${TEST_TMPDIR}/configurer.env"
    [ "$status" -eq 0 ]
}

@test "create_postgresql_user validates pg_stat_statements for existing credentials" {
    load_install_functions
    create_mock_cmd "psql" '
query="$*"
if [[ "$query" == *"SELECT VERSION()"* ]]; then
  exit 0
elif [[ "$query" == *"pg_extension WHERE extname = '"'"'pg_stat_statements'"'"'"* ]]; then
  echo "1"
  exit 0
elif [[ "$query" == *"pg_has_role"* ]]; then
  echo "t"
  exit 0
elif [[ "$query" == *"SELECT datname"* && "$query" == *"pg_database"* ]]; then
  printf "postgres\000"
  exit 0
elif [[ "$query" == *"missing_privilege_count"* ]]; then
  echo "0"
  exit 0
elif [[ "$query" == *"FROM pg_stat_statements LIMIT 0"* ]]; then
  exit 0
elif [[ "$query" == *"pg_extension LIMIT 1"* ]]; then
  exit 0
elif [[ "$query" == *"pg_roles LIMIT 1"* ]]; then
  exit 0
elif [[ "$query" == *"pg_auth_members LIMIT 1"* ]]; then
  exit 0
elif [[ "$query" == *"pg_hba_file_rules LIMIT 1"* ]]; then
  exit 0
elif [[ "$query" == *"has_schema_privilege"* ]]; then
  exit 0
elif [[ "$query" == *"relrowsecurity"* ]]; then
  exit 0
fi
exit 0
'
    run env -u FLAG_PG_STAT_STATEMENTS \
        PATH="${MOCK_BIN}:${PATH}" \
        FLAG_LOG="${TEST_TMPDIR}/pg-stat-flag.log" \
        bash -c '
            RELEEM_TEST_MODE=1 source "$1"
            set +e
            psqlcmd="$2"
            pg_connection_string="-h 127.0.0.1 -p 5432 -d postgres"
            RELEEM_PG_LOGIN="releem"
            RELEEM_PG_PASSWORD="pwd"
            create_postgresql_user
            echo "FLAG_PG_STAT_STATEMENTS=${FLAG_PG_STAT_STATEMENTS:-}" >"${FLAG_LOG}"
        ' _ "${INSTALL_SH}" "${MOCK_BIN}/psql"

    [ "$status" -eq 0 ]
    run grep -E "^FLAG_PG_STAT_STATEMENTS=1$" "${TEST_TMPDIR}/pg-stat-flag.log"
    [ "$status" -eq 0 ]
}

@test "create_postgresql_user grants pg_read_all_data with default root login" {
    create_mock_cmd "psql" '
query="$*"
printf "PGPASSWORD=%s args=%s\n" "${PGPASSWORD-__unset__}" "$query" >> "${PG_ARGS_LOG}"
if [[ "$query" == *"SHOW server_version_num"* ]]; then
  echo "140000"
elif [[ "$query" == *"SELECT datname"* && "$query" == *"pg_database"* ]]; then
  printf "postgres\000shop\000"
elif [[ "$query" == *"GRANT CONNECT ON DATABASE"* ]]; then
  cat >/dev/null
elif [[ "$query" == *"pg_extension WHERE extname = '"'"'pg_stat_statements'"'"'"* ]]; then
  echo "1"
fi
exit 0
'

    run env \
        RELEEM_PG_ROOT_PASSWORD="rootpwd" \
        RELEEM_QUERY_OPTIMIZATION="true" \
        PG_ARGS_LOG="${TEST_TMPDIR}/pg.args" \
        bash -c '
            RELEEM_TEST_MODE=1 source "$1"
            set +e
            psqlcmd="$2"
            pg_root_peer_connection=""
            pg_root_connection_string="-h 127.0.0.1 -p 5432 -d postgres"
            pg_connection_string="-h 127.0.0.1 -p 5432 -d postgres"
            unset RELEEM_PG_LOGIN RELEEM_PG_PASSWORD
            create_postgresql_user
        ' _ "${INSTALL_SH}" "${MOCK_BIN}/psql"

    [ "$status" -eq 0 ]
    run grep -F -- '-v ON_ERROR_STOP=1 -c GRANT CONNECT ON DATABASE "postgres" TO "releem";' "${TEST_TMPDIR}/pg.args"
    [ "$status" -eq 0 ]
    run grep -F -- '-v ON_ERROR_STOP=1 -c GRANT CONNECT ON DATABASE "shop" TO "releem";' "${TEST_TMPDIR}/pg.args"
    [ "$status" -eq 0 ]
    run grep -F -- '-v ON_ERROR_STOP=1 -c GRANT pg_read_all_data TO "releem";' "${TEST_TMPDIR}/pg.args"
    [ "$status" -eq 0 ]
    for procedural_sql in 'DO $$' 'EXECUTE format' 'set_config' 'decode(' 'ALTER DEFAULT PRIVILEGES'; do
        run grep -F -- "${procedural_sql}" "${TEST_TMPDIR}/pg.args"
        [ "$status" -ne 0 ]
    done
}

@test "postgresql role ALTER boundary quotes identifier and binds exact password" {
    create_mock_cmd "psql" '
for argument in "$@"; do
  printf "arg=%s\n" "$argument" >> "${PG_ARGS_LOG}"
done
query="$(cat)"
if [[ -z "$query" ]]; then
  query="${!#}"
fi
query="${query//$'\n'/\\n}"
printf "sql=%s\n" "$query" >> "${PG_ARGS_LOG}"
if [[ "$query" == *"FROM pg_roles"* ]]; then
  echo "1"
fi
exit 0
'

    local role='role"reader'
    local password="pa'ss word; --"
    run env \
        RELEEM_PG_ROOT_PASSWORD="rootpwd" \
        PG_ARGS_LOG="${TEST_TMPDIR}/pg.args" \
        bash -c '
            RELEEM_TEST_MODE=1 source "$1"
            set +e
            psqlcmd="$2"
            pg_root_peer_connection=""
            pg_root_connection_string="-h 127.0.0.1 -p 5432 -d postgres"
            create_or_update_postgresql_monitoring_role "postgres" "$3" "$4"
        ' _ "${INSTALL_SH}" "${MOCK_BIN}/psql" "${role}" "${password}"

    [ "$status" -eq 0 ]
    run grep -F -- "arg=SELECT 1 FROM pg_roles WHERE rolname = 'role\"reader';" "${TEST_TMPDIR}/pg.args"
    [ "$status" -eq 0 ]
    expected_escaped_password="$(printf "%s" "${password}" | sed "s/'/''/g")"
    run grep -F -- "sql=ALTER USER \"role\"\"reader\" WITH PASSWORD " "${TEST_TMPDIR}/pg.args"
    [ "$status" -eq 0 ]
    run grep -F -- "sql=CREATE USER \"role\"\"reader\" WITH PASSWORD '${expected_escaped_password}';" "${TEST_TMPDIR}/pg.args"
    [ "$status" -ne 0 ]
    for grant in \
        'GRANT pg_monitor TO "role""reader";' \
        'GRANT SELECT ON pg_hba_file_rules TO "role""reader";' \
        'GRANT EXECUTE ON FUNCTION pg_hba_file_rules TO "role""reader";' \
        'GRANT EXECUTE ON FUNCTION pg_catalog.pg_reload_conf() TO "role""reader";'; do
        run grep -F -- "arg=${grant}" "${TEST_TMPDIR}/pg.args"
        [ "$status" -eq 0 ]
    done
}

@test "postgresql role CREATE boundary quotes identifier and binds exact password" {
    create_mock_cmd "psql" '
for argument in "$@"; do
  printf "arg=%s\n" "$argument" >> "${PG_ARGS_LOG}"
done
query="$(cat)"
if [[ -z "$query" ]]; then
  query="${!#}"
fi
query="${query//$'\n'/\\n}"
printf "sql=%s\n" "$query" >> "${PG_ARGS_LOG}"
exit 0
'

    local role='new"reader'
    local password="new'pass word; --"
    run env \
        RELEEM_PG_ROOT_PASSWORD="rootpwd" \
        PG_ARGS_LOG="${TEST_TMPDIR}/pg.args" \
        bash -c '
            RELEEM_TEST_MODE=1 source "$1"
            set +e
            psqlcmd="$2"
            pg_root_peer_connection=""
            pg_root_connection_string="-h 127.0.0.1 -p 5432 -d postgres"
            create_or_update_postgresql_monitoring_role "postgres" "$3" "$4"
        ' _ "${INSTALL_SH}" "${MOCK_BIN}/psql" "${role}" "${password}"

    [ "$status" -eq 0 ]
    run grep -F -- "arg=SELECT 1 FROM pg_roles WHERE rolname = 'new\"reader';" "${TEST_TMPDIR}/pg.args"
    [ "$status" -eq 0 ]
    expected_escaped_password="$(printf "%s" "${password}" | sed "s/'/''/g")"
    run grep -F -- "sql=CREATE USER \"new\"\"reader\" WITH PASSWORD " "${TEST_TMPDIR}/pg.args"
    [ "$status" -eq 0 ]
    run grep -F -- "sql=CREATE USER \"new\"\"reader\" WITH PASSWORD '${expected_escaped_password}';" "${TEST_TMPDIR}/pg.args"
    [ "$status" -eq 0 ]
    run grep -F -- "sql=ALTER USER \"new\"\"reader\" WITH PASSWORD '${expected_escaped_password}';" "${TEST_TMPDIR}/pg.args"
    [ "$status" -ne 0 ]
    for grant in \
        'GRANT pg_monitor TO "new""reader";' \
        'GRANT SELECT ON pg_hba_file_rules TO "new""reader";' \
        'GRANT EXECUTE ON FUNCTION pg_hba_file_rules TO "new""reader";' \
        'GRANT EXECUTE ON FUNCTION pg_catalog.pg_reload_conf() TO "new""reader";'; do
        run grep -F -- "arg=${grant}" "${TEST_TMPDIR}/pg.args"
        [ "$status" -eq 0 ]
    done
    run grep -F -- "ALTER USER" "${TEST_TMPDIR}/pg.args"
    [ "$status" -ne 0 ]
}

@test "PostgreSQL commands preserve psql executable and role arguments" {
    create_mock_cmd "psql with spaces" '
user=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -U) user="$2"; shift 2 ;;
    *) shift ;;
  esac
done
printf "user=<%s>\n" "$user" >> "${PG_ARGS_LOG}"
exit 0
'

    run env \
        RELEEM_PG_ROOT_PASSWORD="rootpwd" \
        PG_ARGS_LOG="${TEST_TMPDIR}/pg.args" \
        bash -c '
            RELEEM_TEST_MODE=1 source "$1"
            set +e
            psqlcmd="$2"
            pg_root_peer_connection=""
            pg_root_connection_string="-h 127.0.0.1 -p 5432 -d postgres"
            pg_connection_string="-h 127.0.0.1 -p 5432 -d postgres"
            postgresql_root_exec "root user" -c "SELECT 1;" || true
            RELEEM_PG_LOGIN="monitor user"
            RELEEM_PG_PASSWORD="monitor-password"
            create_postgresql_user || true
        ' _ "${INSTALL_SH}" "${MOCK_BIN}/psql with spaces"

    run grep -Fx -- "user=<root user>" "${TEST_TMPDIR}/pg.args"
    [ "$status" -eq 0 ]
    run grep -Fx -- "user=<monitor user>" "${TEST_TMPDIR}/pg.args"
    [ "$status" -eq 0 ]
}

@test "postgresql_root_exec omits PGPASSWORD for root when root password env is unset" {
    create_mock_cmd "psql" '
printf "PGPASSWORD=%s args=%s\n" "${PGPASSWORD-__unset__}" "$*" >> "${PG_ARGS_LOG}"
exit 0
'

    run env -u RELEEM_PG_ROOT_PASSWORD \
        PG_ARGS_LOG="${TEST_TMPDIR}/pg.args" \
        bash -c '
            RELEEM_TEST_MODE=1 source "$1"
            set +e
            psqlcmd="$2"
            pg_root_peer_connection=""
            pg_root_connection_string="-p 5432 -d postgres"
            postgresql_root_exec postgres -c "SELECT 1;"
        ' _ "${INSTALL_SH}" "${MOCK_BIN}/psql"

    [ "$status" -eq 0 ]
    run grep -F "PGPASSWORD=__unset__" "${TEST_TMPDIR}/pg.args"
    [ "$status" -eq 0 ]
}

@test "postgresql_root_exec keeps empty PGPASSWORD when root password env is set to empty string" {
    create_mock_cmd "psql" '
printf "PGPASSWORD=%s args=%s\n" "${PGPASSWORD-__unset__}" "$*" >> "${PG_ARGS_LOG}"
exit 0
'

    run env \
        RELEEM_PG_ROOT_PASSWORD="" \
        PG_ARGS_LOG="${TEST_TMPDIR}/pg.args" \
        bash -c '
            RELEEM_TEST_MODE=1 source "$1"
            set +e
            psqlcmd="$2"
            pg_root_peer_connection=""
            pg_root_connection_string="-h 127.0.0.1 -p 5432 -d postgres"
            postgresql_root_exec postgres -c "SELECT 1;"
        ' _ "${INSTALL_SH}" "${MOCK_BIN}/psql"

    [ "$status" -eq 0 ]
    run grep -F "PGPASSWORD= args=-h 127.0.0.1 -p 5432 -d postgres -U postgres -c SELECT 1;" "${TEST_TMPDIR}/pg.args"
    [ "$status" -eq 0 ]
}

@test "create_postgresql_user uses custom root login" {
    create_mock_cmd "psql" '
user=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -U) user="$2"; shift 2 ;;
    *) shift ;;
  esac
done
printf "user=%s args=%s\n" "$user" "$*" >> "${PG_ARGS_LOG}"
exit 0
'

    run env \
        RELEEM_PG_ROOT_LOGIN="pgadmin" \
        RELEEM_PG_ROOT_PASSWORD="rootpwd" \
        PG_ARGS_LOG="${TEST_TMPDIR}/pg.args" \
        bash -c '
            RELEEM_TEST_MODE=1 source "$1"
            set +e
            psqlcmd="$2"
            pg_root_peer_connection=""
            pg_root_connection_string="-h 127.0.0.1 -p 5432 -d postgres"
            pg_connection_string="-h 127.0.0.1 -p 5432 -d postgres"
            unset RELEEM_PG_LOGIN RELEEM_PG_PASSWORD
            create_postgresql_user
        ' _ "${INSTALL_SH}" "${MOCK_BIN}/psql"

    [ "$status" -eq 0 ]
    run grep -F "user=pgadmin" "${TEST_TMPDIR}/pg.args"
    [ "$status" -eq 0 ]
}

@test "create_postgresql_user grants pg_read_all_data when query optimization is enabled" {
    create_mock_cmd "psql" '
printf "%s\n" "$*" >> "${PG_ARGS_LOG}"
if [[ "$*" == *"SHOW server_version_num"* ]]; then
  echo "140000"
elif [[ "$*" == *"SELECT datname"* && "$*" == *"pg_database"* ]]; then
  printf "postgres\000shop\000"
fi
exit 0
'

    run env \
        RELEEM_PG_ROOT_LOGIN="pgadmin" \
        RELEEM_PG_ROOT_PASSWORD="rootpwd" \
        RELEEM_QUERY_OPTIMIZATION="true" \
        PG_ARGS_LOG="${TEST_TMPDIR}/pg.args" \
        bash -c '
            RELEEM_TEST_MODE=1 source "$1"
            set +e
            psqlcmd="$2"
            pg_root_peer_connection=""
            pg_root_connection_string="-h 127.0.0.1 -p 5432 -d postgres"
            pg_connection_string="-h 127.0.0.1 -p 5432 -d postgres"
            unset RELEEM_PG_LOGIN RELEEM_PG_PASSWORD
            create_postgresql_user
        ' _ "${INSTALL_SH}" "${MOCK_BIN}/psql"

    [ "$status" -eq 0 ]
    run grep -F -- '-v ON_ERROR_STOP=1 -c GRANT CONNECT ON DATABASE "postgres" TO "releem";' "${TEST_TMPDIR}/pg.args"
    [ "$status" -eq 0 ]
    run grep -F -- '-v ON_ERROR_STOP=1 -c GRANT CONNECT ON DATABASE "shop" TO "releem";' "${TEST_TMPDIR}/pg.args"
    [ "$status" -eq 0 ]
    run grep -F -- '-v ON_ERROR_STOP=1 -c GRANT pg_read_all_data TO "releem";' "${TEST_TMPDIR}/pg.args"
    [ "$status" -eq 0 ]
}

@test "postgresql 12 grants existing objects with explicit schema statements" {
    local pg_sql="${TEST_TMPDIR}/pg.sql"
    local database_catalog_sql=$'SELECT datname\nFROM pg_database\nWHERE datallowconn AND NOT datistemplate\nORDER BY datname;'
    local schema_catalog_sql=$'SELECT nspname\nFROM pg_namespace\nWHERE nspname NOT IN (\'information_schema\', \'pg_catalog\')\n  AND nspname NOT LIKE \'pg_toast%\'\n  AND nspname NOT LIKE \'pg_temp_%\'\nORDER BY nspname;'
    create_mock_cmd "psql" '
query="$*"
printf "%s\n" "$query" >> "${PG_ARGS_LOG}"
printf "%s\0" "${!#}" >> "${PG_SQL_LOG}"
if [[ "$query" == *"SHOW server_version_num"* ]]; then
  echo "120000"
elif [[ "$query" == *"SELECT datname"* && "$query" == *"pg_database"* ]]; then
  printf "postgres\000shop\000"
elif [[ "$query" == *"SELECT nspname"* && "$query" == *"pg_namespace"* ]]; then
  printf "app\000Sales Data\000"
elif [[ "$query" == *"GRANT USAGE ON SCHEMA"* ]]; then
  cat >/dev/null
fi
exit 0
'

    run env \
        RELEEM_PG_ROOT_PASSWORD="rootpwd" \
        PG_ARGS_LOG="${TEST_TMPDIR}/pg.args" \
        PG_SQL_LOG="${pg_sql}" \
        bash -c '
            RELEEM_TEST_MODE=1 source "$1"
            set +e
            psqlcmd="$2"
            pg_root_peer_connection=""
            pg_root_connection_string="-h 127.0.0.1 -p 5432 -d postgres"
            grant_postgresql_query_optimization_access "postgres" "releem"
        ' _ "${INSTALL_SH}" "${MOCK_BIN}/psql"

    [ "$status" -eq 0 ]
    grant_output="$output"
    run assert_exact_sql_payloads "${pg_sql}" \
        'SHOW server_version_num;' \
        "${database_catalog_sql}" \
        'GRANT CONNECT ON DATABASE "postgres" TO "releem";' \
        'GRANT CONNECT ON DATABASE "shop" TO "releem";' \
        "${schema_catalog_sql}" \
        'GRANT USAGE ON SCHEMA "app" TO "releem";' \
        'GRANT SELECT ON ALL TABLES IN SCHEMA "app" TO "releem";' \
        'GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA "app" TO "releem";' \
        'GRANT USAGE ON SCHEMA "Sales Data" TO "releem";' \
        'GRANT SELECT ON ALL TABLES IN SCHEMA "Sales Data" TO "releem";' \
        'GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA "Sales Data" TO "releem";' \
        'ALTER ROLE "releem" IN DATABASE "postgres" SET search_path TO "$user", "public", "app", "Sales Data";' \
        "${schema_catalog_sql}" \
        'GRANT USAGE ON SCHEMA "app" TO "releem";' \
        'GRANT SELECT ON ALL TABLES IN SCHEMA "app" TO "releem";' \
        'GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA "app" TO "releem";' \
        'GRANT USAGE ON SCHEMA "Sales Data" TO "releem";' \
        'GRANT SELECT ON ALL TABLES IN SCHEMA "Sales Data" TO "releem";' \
        'GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA "Sales Data" TO "releem";' \
        'ALTER ROLE "releem" IN DATABASE "shop" SET search_path TO "$user", "public", "app", "Sales Data";'
    [ "$status" -eq 0 ]

    [[ "$grant_output" == *"Rerun the installer after adding PostgreSQL schemas or objects so grants and search_path stay current"* ]]
}

@test "quote_postgresql_identifier escapes embedded double quotes" {
    run bash -c '
        RELEEM_TEST_MODE=1 source "$1"
        quote_postgresql_identifier '"'"'role"reader'"'"'
        printf "\\n"
        quote_postgresql_identifier '"'"'shop"db'"'"'
        printf "\\n"
        quote_postgresql_identifier '"'"'Sales "Data"'"'"'
    ' _ "${INSTALL_SH}"

    [ "$status" -eq 0 ]
    [ "$output" = $'"role""reader"\n"shop""db"\n"Sales ""Data"""' ]
}

@test "postgresql_user_schema_search_path quotes user placeholder and public via quote_postgresql_identifier" {
    run bash -c '
        RELEEM_TEST_MODE=1 source "$1"
        postgresql_user_schema_search_path "$(quote_postgresql_identifier public)" "$(quote_postgresql_identifier app)" "$(quote_postgresql_identifier '"'"'Sales Data'"'"')"
    ' _ "${INSTALL_SH}"

    [ "$status" -eq 0 ]
    [ "$output" = '"$user", "public", "app", "Sales Data"' ]
}

@test "create_postgresql_user prompts for root password when env is unset and passwordless root fails" {
    create_mock_cmd "psql" '
query="$*"
user=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -U) user="$2"; shift 2 ;;
    *) shift ;;
  esac
done
printf "user=%s PGPASSWORD=%s query=%s\n" "$user" "${PGPASSWORD-__unset__}" "$query" >> "${PG_ARGS_LOG}"
if [ "$user" = "postgres" ] && [ "${PGPASSWORD-__unset__}" != "pgprompt" ]; then
  read -r _stolen_password || true
  exit 1
fi
if [[ "$query" == *"pg_extension WHERE extname = '"'"'pg_stat_statements'"'"'"* ]]; then
  echo "1"
fi
exit 0
'

    run env -u RELEEM_PG_ROOT_PASSWORD \
        PG_ARGS_LOG="${TEST_TMPDIR}/pg.args" \
        bash -c '
            RELEEM_TEST_MODE=1 source "$1"
            set +e
            psqlcmd="$2"
            pg_root_peer_connection=""
            pg_root_connection_string="-p 5432 -d postgres"
            pg_connection_string="-h 127.0.0.1 -p 5432 -d postgres"
            unset RELEEM_PG_LOGIN RELEEM_PG_PASSWORD
            create_postgresql_user
        ' _ "${INSTALL_SH}" "${MOCK_BIN}/psql" <<< "pgprompt"

    [ "$status" -eq 0 ]
    run head -n 1 "${TEST_TMPDIR}/pg.args"
    [ "$status" -eq 0 ]
    [[ "$output" == *"user=postgres PGPASSWORD=__unset__"* ]]
    run grep -F "user=postgres PGPASSWORD=pgprompt" "${TEST_TMPDIR}/pg.args"
    [ "$status" -eq 0 ]
}

@test "create_postgresql_user keeps prompting until entered root password works" {
    create_mock_cmd "psql" '
query="$*"
user=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -U) user="$2"; shift 2 ;;
    *) shift ;;
  esac
done
printf "user=%s PGPASSWORD=%s query=%s\n" "$user" "${PGPASSWORD-__unset__}" "$query" >> "${PG_ARGS_LOG}"
if [ "$user" = "postgres" ] && [ "${PGPASSWORD-__unset__}" != "pgprompt" ]; then
  exit 1
fi
if [[ "$query" == *"pg_extension WHERE extname = '"'"'pg_stat_statements'"'"'"* ]]; then
  echo "1"
fi
exit 0
'

    run env -u RELEEM_PG_ROOT_PASSWORD \
        PG_ARGS_LOG="${TEST_TMPDIR}/pg.args" \
        bash -c '
            RELEEM_TEST_MODE=1 source "$1"
            set +e
            psqlcmd="$2"
            pg_root_peer_connection=""
            pg_root_connection_string="-p 5432 -d postgres"
            pg_connection_string="-h 127.0.0.1 -p 5432 -d postgres"
            unset RELEEM_PG_LOGIN RELEEM_PG_PASSWORD
            create_postgresql_user
        ' _ "${INSTALL_SH}" "${MOCK_BIN}/psql" <<< "wrongpg
pgprompt"

    [ "$status" -eq 0 ]
    run grep -F "user=postgres PGPASSWORD=wrongpg" "${TEST_TMPDIR}/pg.args"
    [ "$status" -eq 0 ]
    run grep -F "user=postgres PGPASSWORD=pgprompt" "${TEST_TMPDIR}/pg.args"
    [ "$status" -eq 0 ]
    run grep -F "user=postgres PGPASSWORD=wrongpg" "${TEST_TMPDIR}/pg.args"
    [ "${#lines[@]}" -eq 1 ]
}

@test "windows installer uses conditional MySQL root password args" {
    run grep -F '"-p$RootPassword"' "${REPO_ROOT}/windows/install.ps1"
    [ "$status" -ne 0 ]

    run grep -F "Get-MySQLRootArgs" "${REPO_ROOT}/windows/install.ps1"
    [ "$status" -eq 0 ]

    run grep -F "Prompt-MySQLRootPasswordUntilSuccess" "${REPO_ROOT}/windows/install.ps1"
    [ "$status" -eq 0 ]

    run grep -F "RELEEM_MYSQL_ROOT_LOGIN" "${REPO_ROOT}/windows/install.ps1"
    [ "$status" -eq 0 ]
}

@test "windows installer resolves system utilities without PATH and uses language-neutral principals" {
    run grep -F '[Environment]::GetFolderPath([Environment+SpecialFolder]::Windows)' "${REPO_ROOT}/windows/install.ps1"
    [ "$status" -eq 0 ]

    run grep -F '& icacls ' "${REPO_ROOT}/windows/install.ps1"
    [ "$status" -ne 0 ]

    run grep -F '& powershell.exe ' "${REPO_ROOT}/windows/install.ps1"
    [ "$status" -ne 0 ]

    run grep -F "'System32\WindowsPowerShell\v1.0\powershell.exe'" "${REPO_ROOT}/windows/install.ps1"
    [ "$status" -eq 0 ]

    run grep -F "'*S-1-5-18:(R)'" "${REPO_ROOT}/windows/install.ps1"
    [ "$status" -eq 0 ]

    run grep -F "'*S-1-5-32-544:(M)'" "${REPO_ROOT}/windows/install.ps1"
    [ "$status" -eq 0 ]

    run grep -F 'if ($LASTEXITCODE -ne 0)' "${REPO_ROOT}/windows/install.ps1"
    [ "$status" -eq 0 ]
}

@test "create_mysql_user omits root password option when root password env is unset and root connects" {
    load_install_functions
    set -e
    create_mock_cmd "mysqladmin" '
printf "%s\n" "$*" >> "${MYSQLADMIN_ARGS_LOG}"
echo "mysqld is alive"
'
    create_mock_cmd "mysql" '
printf "%s\n" "$*" >> "${MYSQL_ARGS_LOG}"
exit 0
'

    export MYSQLADMIN_ARGS_LOG="${TEST_TMPDIR}/mysqladmin.args"
    export MYSQL_ARGS_LOG="${TEST_TMPDIR}/mysql.args"
    mysqladmincmd="${MOCK_BIN}/mysqladmin"
    mysqlcmd="${MOCK_BIN}/mysql"
    root_connection_string="--host=127.0.0.1 --port=3306"
    connection_string="--host=127.0.0.1 --port=3306"
    mysql_user_host="127.0.0.1"
    unset RELEEM_MYSQL_ROOT_PASSWORD RELEEM_MYSQL_LOGIN RELEEM_MYSQL_PASSWORD RELEEM_QUERY_OPTIMIZATION

    run create_mysql_user

    [ "$status" -eq 0 ] || return 1
    run grep -F -e "--user=root --password= ping" "${TEST_TMPDIR}/mysqladmin.args"
    [ "$status" -ne 0 ] || return 1
}

@test "create_mysql_user keeps empty root password option when root password env is set to empty string" {
    load_install_functions
    set -e
    create_mock_cmd "mysqladmin" '
printf "%s\n" "$*" >> "${MYSQLADMIN_ARGS_LOG}"
echo "mysqld is alive"
'
    create_mock_cmd "mysql" '
printf "%s\n" "$*" >> "${MYSQL_ARGS_LOG}"
exit 0
'

    export MYSQLADMIN_ARGS_LOG="${TEST_TMPDIR}/mysqladmin.args"
    export MYSQL_ARGS_LOG="${TEST_TMPDIR}/mysql.args"
    mysqladmincmd="${MOCK_BIN}/mysqladmin"
    mysqlcmd="${MOCK_BIN}/mysql"
    root_connection_string="--host=127.0.0.1 --port=3306"
    connection_string="--host=127.0.0.1 --port=3306"
    mysql_user_host="127.0.0.1"
    RELEEM_MYSQL_ROOT_PASSWORD=""
    unset RELEEM_MYSQL_LOGIN RELEEM_MYSQL_PASSWORD RELEEM_QUERY_OPTIMIZATION

    run create_mysql_user

    [ "$status" -eq 0 ] || return 1
    run head -n 1 "${TEST_TMPDIR}/mysqladmin.args"
    [ "$status" -eq 0 ] || return 1
    [[ "$output" == *"--password="* ]] || return 1
}

@test "create_mysql_user uses custom root login" {
    load_install_functions
    set -e
    create_mock_cmd "mysqladmin" '
printf "%s\n" "$*" >> "${MYSQLADMIN_ARGS_LOG}"
echo "mysqld is alive"
'
    create_mock_cmd "mysql" '
printf "%s\n" "$*" >> "${MYSQL_ARGS_LOG}"
exit 0
'

    export MYSQLADMIN_ARGS_LOG="${TEST_TMPDIR}/mysqladmin.args"
    export MYSQL_ARGS_LOG="${TEST_TMPDIR}/mysql.args"
    mysqladmincmd="${MOCK_BIN}/mysqladmin"
    mysqlcmd="${MOCK_BIN}/mysql"
    root_connection_string="--host=127.0.0.1 --port=3306"
    connection_string="--host=127.0.0.1 --port=3306"
    mysql_user_host="127.0.0.1"
    RELEEM_MYSQL_ROOT_LOGIN="admin"
    unset RELEEM_MYSQL_ROOT_PASSWORD RELEEM_MYSQL_LOGIN RELEEM_MYSQL_PASSWORD RELEEM_QUERY_OPTIMIZATION

    run create_mysql_user

    [ "$status" -eq 0 ] || return 1
    run grep -F -- "--user=admin" "${TEST_TMPDIR}/mysqladmin.args"
    [ "$status" -eq 0 ] || return 1
    run grep -F -- "--user=admin" "${TEST_TMPDIR}/mysql.args"
    [ "$status" -eq 0 ] || return 1
}

@test "create_mysql_user prompts for root password when env is unset and passwordless root fails" {
    load_install_functions
    set -e
    create_mock_cmd "mysqladmin" '
printf "%s\n" "$*" >> "${MYSQLADMIN_ARGS_LOG}"
if [[ "$*" == *"--user=root"* && "$*" != *"--password=promptedpwd"* ]]; then
  exit 1
fi
echo "mysqld is alive"
'
    create_mock_cmd "mysql" '
printf "%s\n" "$*" >> "${MYSQL_ARGS_LOG}"
if [[ "$*" == *"--user=root"* && "$*" != *"--password=promptedpwd"* ]]; then
  exit 1
fi
exit 0
'

    export MYSQLADMIN_ARGS_LOG="${TEST_TMPDIR}/mysqladmin.args"
    export MYSQL_ARGS_LOG="${TEST_TMPDIR}/mysql.args"
    mysqladmincmd="${MOCK_BIN}/mysqladmin"
    mysqlcmd="${MOCK_BIN}/mysql"
    root_connection_string="--host=127.0.0.1 --port=3306"
    connection_string="--host=127.0.0.1 --port=3306"
    mysql_user_host="127.0.0.1"
    unset RELEEM_MYSQL_ROOT_PASSWORD RELEEM_MYSQL_LOGIN RELEEM_MYSQL_PASSWORD RELEEM_QUERY_OPTIMIZATION

    run create_mysql_user <<< "promptedpwd"

    [ "$status" -eq 0 ] || return 1
    run grep -F -e "--user=root --password=promptedpwd ping" "${TEST_TMPDIR}/mysqladmin.args"
    [ "$status" -eq 0 ] || return 1
    run grep -F -e "--user=root --password=promptedpwd -Be CREATE USER" "${TEST_TMPDIR}/mysql.args"
    [ "$status" -eq 0 ] || return 1
}

@test "create_mysql_user keeps prompting until entered root password works" {
    load_install_functions
    set -e
    create_mock_cmd "mysqladmin" '
printf "%s\n" "$*" >> "${MYSQLADMIN_ARGS_LOG}"
if [[ "$*" == *"--user=root"* && "$*" != *"--password=promptedpwd"* ]]; then
  exit 1
fi
echo "mysqld is alive"
'
    create_mock_cmd "mysql" '
printf "%s\n" "$*" >> "${MYSQL_ARGS_LOG}"
if [[ "$*" == *"--user=root"* && "$*" != *"--password=promptedpwd"* ]]; then
  exit 1
fi
exit 0
'

    export MYSQLADMIN_ARGS_LOG="${TEST_TMPDIR}/mysqladmin.args"
    export MYSQL_ARGS_LOG="${TEST_TMPDIR}/mysql.args"
    mysqladmincmd="${MOCK_BIN}/mysqladmin"
    mysqlcmd="${MOCK_BIN}/mysql"
    root_connection_string="--host=127.0.0.1 --port=3306"
    connection_string="--host=127.0.0.1 --port=3306"
    mysql_user_host="127.0.0.1"
    unset RELEEM_MYSQL_ROOT_PASSWORD RELEEM_MYSQL_LOGIN RELEEM_MYSQL_PASSWORD RELEEM_QUERY_OPTIMIZATION

    run create_mysql_user <<< "wrongpwd
promptedpwd"

    [ "$status" -eq 0 ] || return 1
    run grep -F -e "--user=root --password=promptedpwd ping" "${TEST_TMPDIR}/mysqladmin.args"
    [ "$status" -eq 0 ] || return 1
    run grep -F -e "--user=root --password=promptedpwd -Be CREATE USER" "${TEST_TMPDIR}/mysql.args"
    [ "$status" -eq 0 ] || return 1
}

@test "detect_mysql_commands exits non-zero when mysql binaries are missing" {
    create_mock_cmd "which" 'exit 1'
    run env RELEEM_TEST_MODE=1 PATH="${MOCK_BIN}:/bin" bash -c "source '${INSTALL_SH}'; detect_mysql_commands"
    [ "$status" -ne 0 ]
    [[ "$output" == *"Couldn't find mysqladmin/mariadb-admin"* ]]
}

@test "Aurora aws/rds mode writes both required parameter groups to releem.conf" {
    prepare_common_install_mocks
    local workdir="${TEST_TMPDIR}/workdir"
    local conf="${workdir}/releem.conf"
    mkdir -p "${workdir}"
    PATH="${MOCK_BIN}:${PATH}" run env \
        RELEEM_TEST_MODE=1 \
        RELEEM_WORKDIR="${workdir}" \
        RELEEM_CONF_FILE="${conf}" \
        RELEEM_API_KEY="k1" \
        RELEEM_INSTANCE_TYPE="aws/rds" \
        RELEEM_AWS_REGION="eu-west-1" \
        RELEEM_AWS_RDS_DB="db-1" \
        RELEEM_AWS_RDS_PARAMETER_GROUP="releem-agent" \
        RELEEM_AWS_RDS_CLUSTER_PARAMETER_GROUP="aurora-cluster-custom" \
        RELEEM_MYSQL_LOGIN="releem" \
        RELEEM_MYSQL_PASSWORD="pwd" \
        RELEEM_DB_MEMORY_LIMIT="0" \
        RELEEM_CRON_ENABLE="1" \
        RELEEM_AGENT_DISABLE="1" \
        bash "${INSTALL_SH}"

    [ "$status" -eq 0 ]
    run grep -E "^(releem_dir|instance_type|aws_region|aws_rds_db|aws_rds_parameter_group|aws_rds_cluster_parameter_group)=" "${conf}"
    [ "$status" -eq 0 ]
    [[ "$output" == *"releem_dir=\"${workdir}\""* ]]
    [[ "$output" == *'instance_type="aws/rds"'* ]]
    [[ "$output" == *'aws_region="eu-west-1"'* ]]
    [[ "$output" == *'aws_rds_cluster_parameter_group="aurora-cluster-custom"'* ]]
}

@test "ordinary non-Aurora RDS mode does not write a cluster parameter group" {
    prepare_common_install_mocks
    local workdir="${TEST_TMPDIR}/workdir"
    local conf="${workdir}/releem.conf"
    mkdir -p "${workdir}"
    PATH="${MOCK_BIN}:${PATH}" run env \
        RELEEM_TEST_MODE=1 \
        RELEEM_WORKDIR="${workdir}" \
        RELEEM_CONF_FILE="${conf}" \
        RELEEM_API_KEY="k1" \
        RELEEM_INSTANCE_TYPE="aws/rds" \
        RELEEM_AWS_REGION="eu-west-1" \
        RELEEM_AWS_RDS_DB="db-1" \
        RELEEM_AWS_RDS_PARAMETER_GROUP="releem-agent" \
        RELEEM_MYSQL_LOGIN="releem" \
        RELEEM_MYSQL_PASSWORD="pwd" \
        RELEEM_DB_MEMORY_LIMIT="0" \
        RELEEM_CRON_ENABLE="1" \
        RELEEM_AGENT_DISABLE="1" \
        bash "${INSTALL_SH}"

    [ "$status" -eq 0 ]
    run grep -F 'aws_rds_cluster_parameter_group=' "${conf}"
    [ "$status" -eq 1 ]
}

@test "aws/rds mode fails when mandatory aws vars are missing" {
    prepare_common_install_mocks
    local workdir="${TEST_TMPDIR}/workdir"
    local conf="${workdir}/releem.conf"
    mkdir -p "${workdir}"
    PATH="${MOCK_BIN}:${PATH}" run env \
        RELEEM_TEST_MODE=1 \
        RELEEM_WORKDIR="${workdir}" \
        RELEEM_CONF_FILE="${conf}" \
        RELEEM_API_KEY="k1" \
        RELEEM_INSTANCE_TYPE="aws/rds" \
        RELEEM_MYSQL_LOGIN="releem" \
        RELEEM_MYSQL_PASSWORD="pwd" \
        RELEEM_DB_MEMORY_LIMIT="0" \
        RELEEM_CRON_ENABLE="1" \
        RELEEM_AGENT_DISABLE="1" \
        bash "${INSTALL_SH}"

    [ "$status" -eq 1 ]
    [[ "$output" == *"AWS region, AWS RDS DB or AWS RDS Parameter Group is not set"* ]]
}

@test "gcp/cloudsql mode writes gcp keys and query optimization flag" {
    prepare_common_install_mocks
    local workdir="${TEST_TMPDIR}/workdir"
    local conf="${workdir}/releem.conf"
    mkdir -p "${workdir}"
    PATH="${MOCK_BIN}:${PATH}" run env \
        RELEEM_TEST_MODE=1 \
        RELEEM_WORKDIR="${workdir}" \
        RELEEM_CONF_FILE="${conf}" \
        RELEEM_API_KEY="k1" \
        RELEEM_INSTANCE_TYPE="gcp/cloudsql" \
        RELEEM_GCP_PROJECT_ID="project-1" \
        RELEEM_GCP_REGION="us-central1" \
        RELEEM_GCP_CLOUDSQL_INSTANCE="inst-1" \
        RELEEM_MYSQL_LOGIN="releem" \
        RELEEM_MYSQL_PASSWORD="pwd" \
        RELEEM_QUERY_OPTIMIZATION="true" \
        RELEEM_DB_MEMORY_LIMIT="0" \
        RELEEM_CRON_ENABLE="1" \
        RELEEM_AGENT_DISABLE="1" \
        bash "${INSTALL_SH}"

    [ "$status" -eq 0 ]
    run grep -E "^(instance_type|gcp_project_id|gcp_region|gcp_cloudsql_instance|query_optimization)=" "${conf}"
    [ "$status" -eq 0 ]
    [[ "$output" == *'instance_type="gcp/cloudsql"'* ]]
    [[ "$output" == *'query_optimization=true'* ]]
}

@test "azure/mysql mode writes azure keys and enables ssl by default" {
    prepare_common_install_mocks
    local workdir="${TEST_TMPDIR}/workdir"
    local conf="${workdir}/releem.conf"
    mkdir -p "${workdir}"
    PATH="${MOCK_BIN}:${PATH}" run env \
        RELEEM_TEST_MODE=1 \
        RELEEM_WORKDIR="${workdir}" \
        RELEEM_CONF_FILE="${conf}" \
        RELEEM_API_KEY="k1" \
        RELEEM_INSTANCE_TYPE="azure/mysql" \
        RELEEM_AZURE_SUBSCRIPTION_ID="sub-1" \
        RELEEM_AZURE_RESOURCE_GROUP="rg-1" \
        RELEEM_AZURE_MYSQL_SERVER="mysql-1" \
        RELEEM_MYSQL_LOGIN="releem" \
        RELEEM_MYSQL_PASSWORD="pwd" \
        RELEEM_DB_MEMORY_LIMIT="0" \
        RELEEM_CRON_ENABLE="1" \
        RELEEM_AGENT_DISABLE="1" \
        bash "${INSTALL_SH}"

    [ "$status" -eq 0 ]
    run grep -E "^(instance_type|azure_subscription_id|azure_resource_group|azure_mysql_server|mysql_ssl_mode)=" "${conf}"
    [ "$status" -eq 0 ]
    [[ "$output" == *'instance_type="azure/mysql"'* ]]
    [[ "$output" == *'azure_subscription_id="sub-1"'* ]]
    [[ "$output" == *'azure_resource_group="rg-1"'* ]]
    [[ "$output" == *'azure_mysql_server="mysql-1"'* ]]
    [[ "$output" == *'mysql_ssl_mode=true'* ]]
}

@test "azure/mysql mode fails when mandatory azure vars are missing" {
    prepare_common_install_mocks
    local workdir="${TEST_TMPDIR}/workdir"
    local conf="${workdir}/releem.conf"
    mkdir -p "${workdir}"
    PATH="${MOCK_BIN}:${PATH}" run env \
        RELEEM_TEST_MODE=1 \
        RELEEM_WORKDIR="${workdir}" \
        RELEEM_CONF_FILE="${conf}" \
        RELEEM_API_KEY="k1" \
        RELEEM_INSTANCE_TYPE="azure/mysql" \
        RELEEM_MYSQL_LOGIN="releem" \
        RELEEM_MYSQL_PASSWORD="pwd" \
        RELEEM_DB_MEMORY_LIMIT="0" \
        RELEEM_CRON_ENABLE="1" \
        RELEEM_AGENT_DISABLE="1" \
        bash "${INSTALL_SH}"

    [ "$status" -eq 1 ]
    [[ "$output" == *"Azure subscription ID, resource group or MySQL server is not set"* ]]
}

@test "enable_query_optimization mode updates releem.conf and executes releem" {
    prepare_common_install_mocks
    create_mock_cmd "mysqladmin" 'echo "mysqld is alive"'
    create_mock_cmd "mariadb-admin" 'echo "mysqld is alive"'
    create_mock_cmd "mysql" '
printf "%s\n" "$*" >> "${MYSQL_ARGS_LOG}"
if [[ "$*" == *"-NBe select Concat("*"from mysql.user where HEX(User)="* ]]; then
  echo "GRANT SELECT on *.* to \`releem\`@\`%\`;"
elif [[ "$*" == *"-Be GRANT SELECT on *.* to"* ]]; then
  exit 0
elif [[ "$*" == *"--user=root"* ]]; then
  exit 64
fi
exit 0
'
    create_mock_cmd "mariadb" '
printf "%s\n" "$*" >> "${MYSQL_ARGS_LOG}"
if [[ "$*" == *"-NBe select Concat("*"from mysql.user where HEX(User)="* ]]; then
  echo "GRANT SELECT on *.* to \`releem\`@\`%\`;"
elif [[ "$*" == *"-Be GRANT SELECT on *.* to"* ]]; then
  exit 0
elif [[ "$*" == *"--user=root"* ]]; then
  exit 64
fi
exit 0
'

    local workdir="${TEST_TMPDIR}/workdir"
    local conf="${workdir}/releem.conf"
    local mysql_args="${TEST_TMPDIR}/mysql.args"
    mkdir -p "${workdir}"
    printf 'apikey="k1"\n' > "${conf}"
    printf '#!/usr/bin/env bash\nexit 0\n' > "${workdir}/releem-agent"
    printf '#!/usr/bin/env bash\necho "$@" >> "%s/calls.log"\nexit 0\n' "${workdir}" > "${workdir}/releem.sh"
    chmod +x "${workdir}/releem-agent" "${workdir}/releem.sh"
    cp "${INSTALL_SH}" "${MOCK_BIN}/enable_query_optimization"
    chmod +x "${MOCK_BIN}/enable_query_optimization"

    PATH="${MOCK_BIN}:${PATH}" run env \
        RELEEM_TEST_MODE=1 \
        RELEEM_WORKDIR="${workdir}" \
        RELEEM_CONF_FILE="${conf}" \
        RELEEM_API_KEY="k1" \
        RELEEM_MYSQL_ROOT_LOGIN="admin" \
        RELEEM_MYSQL_ROOT_PASSWORD="rootpwd" \
        MYSQL_ARGS_LOG="${mysql_args}" \
        RELEEM_CRON_ENABLE="1" \
        enable_query_optimization

    [ "$status" -eq 0 ]
    run grep -F -- "--user=admin" "${mysql_args}"
    [ "$status" -eq 0 ]
    run grep -F -- "-NBe select Concat(" "${mysql_args}"
    [ "$status" -eq 0 ]
    run grep -F -- "-Be GRANT SELECT on *.* to" "${mysql_args}"
    [ "$status" -eq 0 ]
    run grep -E "^query_optimization=true$" "${conf}"
    [ "$status" -eq 0 ]
    run grep -E "^-p$" "${workdir}/calls.log"
    [ "$status" -eq 0 ]
}

run_query_optimization_configurer_fixture() {
    local database="$1"
    local scripts="$2"
    prepare_common_install_mocks
    create_mock_cmd "sleep" 'exit 0'
    create_mock_cmd "curl" 'echo "unexpected download" >&2; exit 92'
    create_mock_cmd "mysqladmin" 'echo "mysqld is alive"'
    create_mock_cmd "mariadb-admin" 'echo "mysqld is alive"'
    create_mock_cmd "mysql" '
if [[ "$*" == *"-NBe select Concat("* ]]; then
  echo "GRANT SELECT on *.* to \`releem\`@\`%\`;"
fi
exit 0
'
    cp "${MOCK_BIN}/mysql" "${MOCK_BIN}/mariadb"
    create_mock_cmd "psql" '
if [[ "$*" == *"SHOW server_version_num"* ]]; then
  echo "140000"
elif [[ "$*" == *"SELECT datname"* && "$*" == *"pg_database"* ]]; then
  printf "postgres\000shop\000"
fi
exit 0
'
    local workdir="${TEST_TMPDIR}/workdir"
    mkdir -p "${workdir}"
    printf 'apikey="k1"\nquery_optimization=false\n' > "${workdir}/releem.conf"
    printf '#!/usr/bin/env bash\nexit 0\n' > "${workdir}/releem-agent"
    chmod +x "${workdir}/releem-agent"
    cat > "${workdir}/mysqlconfigurer.sh" <<'EOF'
#!/usr/bin/env bash
printf '%s:%s\n' "$(basename "$0")" "$*" >> "$CONFIGURER_CALLS"
EOF
    if [ "$scripts" = both ]; then
        cp "${workdir}/mysqlconfigurer.sh" "${workdir}/releem.sh"
    fi
    local -a database_env
    if [ "$database" = postgresql ]; then
        database_env=(RELEEM_PG_ROOT_LOGIN=pgadmin RELEEM_PG_ROOT_PASSWORD=rootpwd)
    else
        database_env=(RELEEM_MYSQL_ROOT_LOGIN=admin RELEEM_MYSQL_ROOT_PASSWORD=rootpwd)
    fi
    PATH="${MOCK_BIN}:${PATH}" run env \
        PATH="${MOCK_BIN}:${PATH}" \
        RELEEM_TEST_MODE=1 \
        RELEEM_WORKDIR="${workdir}" \
        RELEEM_API_KEY=k1 \
        CONFIGURER_CALLS="${workdir}/calls.log" \
        "${database_env[@]}" \
        bash "${INSTALL_SH}" enable_query_optimization
    if [ "$status" -ne 0 ]; then
        printf '%s\n' "$output"
        return 1
    fi
    grep -qx 'query_optimization=true' "${workdir}/releem.conf"
}

@test "enable_query_optimization uses the legacy configurer on legacy-only MySQL installs" {
    run_query_optimization_configurer_fixture mysql legacy
    [ "$(cat "${TEST_TMPDIR}/workdir/calls.log")" = 'mysqlconfigurer.sh:-p' ]
    [ ! -e "${TEST_TMPDIR}/workdir/releem.sh" ]
}

@test "enable_query_optimization uses the legacy configurer on legacy-only PostgreSQL installs" {
    run_query_optimization_configurer_fixture postgresql legacy
    [ "$(cat "${TEST_TMPDIR}/workdir/calls.log")" = 'mysqlconfigurer.sh:-p' ]
    [ ! -e "${TEST_TMPDIR}/workdir/releem.sh" ]
}

@test "enable_query_optimization prefers the canonical configurer when both scripts exist" {
    run_query_optimization_configurer_fixture mysql both
    [ "$(cat "${TEST_TMPDIR}/workdir/calls.log")" = 'releem.sh:-p' ]
}

@test "enable_query_optimization mode prompts for root password when env is unset" {
    prepare_common_install_mocks
    create_mock_cmd "mysqladmin" '
printf "%s\n" "$*" >> "${MYSQLADMIN_ARGS_LOG}"
if [[ "$*" == *"--user=root"* && "$*" != *"--password=promptedpwd"* ]]; then
  exit 1
fi
echo "mysqld is alive"
'
    create_mock_cmd "mariadb-admin" '
printf "%s\n" "$*" >> "${MYSQLADMIN_ARGS_LOG}"
if [[ "$*" == *"--user=root"* && "$*" != *"--password=promptedpwd"* ]]; then
  exit 1
fi
echo "mysqld is alive"
'
    create_mock_cmd "mysql" '
printf "%s\n" "$*" >> "${MYSQL_ARGS_LOG}"
if [[ "$*" == *"--user=root"* && "$*" != *"--password=promptedpwd"* ]]; then
  exit 1
fi
if [[ "$*" == *"-NBe select Concat("*"from mysql.user where HEX(User)="* ]]; then
  echo "GRANT SELECT on *.* to \`releem\`@\`%\`;"
fi
exit 0
'
    create_mock_cmd "mariadb" '
printf "%s\n" "$*" >> "${MYSQL_ARGS_LOG}"
if [[ "$*" == *"--user=root"* && "$*" != *"--password=promptedpwd"* ]]; then
  exit 1
fi
if [[ "$*" == *"-NBe select Concat("*"from mysql.user where HEX(User)="* ]]; then
  echo "GRANT SELECT on *.* to \`releem\`@\`%\`;"
fi
exit 0
'

    local workdir="${TEST_TMPDIR}/workdir"
    local conf="${workdir}/releem.conf"
    local mysqladmin_args="${TEST_TMPDIR}/mysqladmin.args"
    local mysql_args="${TEST_TMPDIR}/mysql.args"
    mkdir -p "${workdir}"
    printf 'apikey="k1"\n' > "${conf}"
    printf '#!/usr/bin/env bash\nexit 0\n' > "${workdir}/releem-agent"
    printf '#!/usr/bin/env bash\necho "$@" >> "%s/calls.log"\nexit 0\n' "${workdir}" > "${workdir}/releem.sh"
    chmod +x "${workdir}/releem-agent" "${workdir}/releem.sh"
    cp "${INSTALL_SH}" "${MOCK_BIN}/enable_query_optimization"
    chmod +x "${MOCK_BIN}/enable_query_optimization"

    PATH="${MOCK_BIN}:${PATH}" run env -u RELEEM_MYSQL_ROOT_PASSWORD \
        RELEEM_TEST_MODE=1 \
        RELEEM_WORKDIR="${workdir}" \
        RELEEM_CONF_FILE="${conf}" \
        RELEEM_API_KEY="k1" \
        MYSQLADMIN_ARGS_LOG="${mysqladmin_args}" \
        MYSQL_ARGS_LOG="${mysql_args}" \
        RELEEM_CRON_ENABLE="1" \
        enable_query_optimization <<< "promptedpwd"

    [ "$status" -eq 0 ]
    run grep -F -- "--user=root ping" "${mysqladmin_args}"
    [ "$status" -eq 0 ]
    run grep -F -- "--user=root --password=promptedpwd ping" "${mysqladmin_args}"
    [ "$status" -eq 0 ]
    run grep -F -- "--user=root --password=promptedpwd -NBe select Concat(" "${mysql_args}"
    [ "$status" -eq 0 ]
    run grep -F -- "--user=root --password=promptedpwd -Be GRANT SELECT on *.* to" "${mysql_args}"
    [ "$status" -eq 0 ]
    run grep -E "^query_optimization=true$" "${conf}"
    [ "$status" -eq 0 ]
    run grep -E "^-p$" "${workdir}/calls.log"
    [ "$status" -eq 0 ]
}

@test "enable_query_optimization mode uses existing PostgreSQL config without env database hints" {
    local pg_sql="${TEST_TMPDIR}/pg.sql"
    local database_catalog_sql=$'SELECT datname\nFROM pg_database\nWHERE datallowconn AND NOT datistemplate\nORDER BY datname;'
    local schema_catalog_sql=$'SELECT nspname\nFROM pg_namespace\nWHERE nspname NOT IN (\'information_schema\', \'pg_catalog\')\n  AND nspname NOT LIKE \'pg_toast%\'\n  AND nspname NOT LIKE \'pg_temp_%\'\nORDER BY nspname;'
    prepare_common_install_mocks
    create_mock_cmd "sudo" '
if [ "$1" = "-u" ]; then
  shift 2
fi
"$@"
'
    create_mock_cmd "psql" '
printf "%s\n" "$*" >> "${PG_ARGS_LOG}"
printf "%s\0" "${!#}" >> "${PG_SQL_LOG}"
if [[ "$*" == *"SHOW server_version_num"* ]]; then
  echo "140000"
elif [[ "$*" == *"SELECT datname"* && "$*" == *"pg_database"* ]]; then
  printf "postgres\000shop\000"
elif [[ "$*" == *"SELECT nspname"* && "$*" == *"pg_namespace"* ]]; then
  printf ""
fi
exit 0
'
    create_mock_cmd "mysqladmin" '
echo "unexpected mysqladmin call: $*" >> "${MYSQL_ARGS_LOG}"
exit 44
'
    create_mock_cmd "mariadb-admin" '
echo "unexpected mariadb-admin call: $*" >> "${MYSQL_ARGS_LOG}"
exit 44
'
    create_mock_cmd "mysql" '
echo "unexpected mysql call: $*" >> "${MYSQL_ARGS_LOG}"
exit 44
'
    create_mock_cmd "mariadb" '
echo "unexpected mariadb call: $*" >> "${MYSQL_ARGS_LOG}"
exit 44
'

    local workdir="${TEST_TMPDIR}/workdir"
    local conf="${workdir}/releem.conf"
    local pg_args="${TEST_TMPDIR}/pg.args"
    local mysql_args="${TEST_TMPDIR}/mysql.args"
    mkdir -p "${workdir}"
    printf 'apikey="k1"\npg_user="collector-ro"\npg_password="secret"\npg_host="127.0.0.1"\npg_port="5432"\n' > "${conf}"
    printf '#!/usr/bin/env bash\nexit 0\n' > "${workdir}/releem-agent"
    printf '#!/usr/bin/env bash\necho "$@" >> "%s/calls.log"\nexit 0\n' "${workdir}" > "${workdir}/releem.sh"
    chmod +x "${workdir}/releem-agent" "${workdir}/releem.sh"
    cp "${INSTALL_SH}" "${MOCK_BIN}/enable_query_optimization"
    chmod +x "${MOCK_BIN}/enable_query_optimization"

    PATH="${MOCK_BIN}:${PATH}" run env -u RELEEM_PG_HOST -u RELEEM_PG_LOGIN -u RELEEM_PG_PASSWORD -u RELEEM_PG_ROOT_LOGIN -u RELEEM_PG_ROOT_PASSWORD -u RELEEM_PG_TYPE -u RELEEM_MYSQL_HOST -u RELEEM_MYSQL_LOGIN -u RELEEM_MYSQL_PASSWORD -u RELEEM_MYSQL_ROOT_LOGIN -u RELEEM_MYSQL_ROOT_PASSWORD -u RELEEM_MYSQL_TYPE \
        RELEEM_TEST_MODE=1 \
        RELEEM_WORKDIR="${workdir}" \
        RELEEM_CONF_FILE="${conf}" \
        PG_ARGS_LOG="${pg_args}" \
        PG_SQL_LOG="${pg_sql}" \
        MYSQL_ARGS_LOG="${mysql_args}" \
        RELEEM_CRON_ENABLE="1" \
        bash -x "${MOCK_BIN}/enable_query_optimization" enable_query_optimization

    [ "$status" -eq 0 ]
    run assert_exact_sql_payloads "${pg_sql}" \
        'SELECT VERSION()' \
        'SELECT VERSION()' \
        'SHOW server_version_num;' \
        "${database_catalog_sql}" \
        'GRANT CONNECT ON DATABASE "postgres" TO "collector-ro";' \
        'GRANT CONNECT ON DATABASE "shop" TO "collector-ro";' \
        'GRANT pg_read_all_data TO "collector-ro";' \
        "${schema_catalog_sql}" \
        'ALTER ROLE "collector-ro" IN DATABASE "postgres" SET search_path TO "$user", "public";' \
        "${schema_catalog_sql}" \
        'ALTER ROLE "collector-ro" IN DATABASE "shop" SET search_path TO "$user", "public";'
    [ "$status" -eq 0 ]
    [ ! -s "${mysql_args}" ]
    run grep -E "^query_optimization=true$" "${conf}"
    [ "$status" -eq 0 ]
}

@test "enable_query_optimization mode grants MySQL access to configured monitoring user" {
    local mysql_sql="${TEST_TMPDIR}/mysql.sql"
    local account_catalog_sql="select Concat('GRANT SELECT on *.* to ', CHAR(96), REPLACE(User, CHAR(96), CONCAT(CHAR(96), CHAR(96))), CHAR(96), '@', CHAR(96), REPLACE(Host, CHAR(96), CONCAT(CHAR(96), CHAR(96))), CHAR(96), ';') from mysql.user where HEX(User)='6F277265696C6C79'"
    prepare_common_install_mocks
    create_mock_cmd "mysqladmin" 'echo "mysqld is alive"'
    create_mock_cmd "mariadb-admin" 'echo "mysqld is alive"'
    create_mock_cmd "mysql" '
printf "%s\n" "$*" >> "${MYSQL_ARGS_LOG}"
printf "%s\0" "${!#}" >> "${MYSQL_SQL_LOG}"
if [[ "$*" == *"HEX(User)="*"6F277265696C6C79"* ]]; then
  printf "%s\\n" "GRANT SELECT on *.* to \`o'"'"'reilly\`@\`localhost\`;"
fi
exit 0
'
    create_mock_cmd "mariadb" '
printf "%s\n" "$*" >> "${MYSQL_ARGS_LOG}"
printf "%s\0" "${!#}" >> "${MYSQL_SQL_LOG}"
if [[ "$*" == *"HEX(User)="*"6F277265696C6C79"* ]]; then
  printf "%s\\n" "GRANT SELECT on *.* to \`o'"'"'reilly\`@\`localhost\`;"
fi
exit 0
'

    local workdir="${TEST_TMPDIR}/workdir"
    local conf="${workdir}/releem.conf"
    local mysql_args="${TEST_TMPDIR}/mysql.args"
    mkdir -p "${workdir}"
    printf 'apikey="k1"\nmysql_user="o'"'"'reilly"\nmysql_password="secret"\nmysql_host="127.0.0.1"\nmysql_port="3306"\n' > "${conf}"
    printf '#!/usr/bin/env bash\nexit 0\n' > "${workdir}/releem-agent"
    printf '#!/usr/bin/env bash\necho "$@" >> "%s/calls.log"\nexit 0\n' "${workdir}" > "${workdir}/releem.sh"
    chmod +x "${workdir}/releem-agent" "${workdir}/releem.sh"
    cp "${INSTALL_SH}" "${MOCK_BIN}/enable_query_optimization"
    chmod +x "${MOCK_BIN}/enable_query_optimization"

    PATH="${MOCK_BIN}:${PATH}" run env -u RELEEM_PG_HOST -u RELEEM_PG_LOGIN -u RELEEM_PG_PASSWORD -u RELEEM_PG_ROOT_LOGIN -u RELEEM_PG_ROOT_PASSWORD -u RELEEM_PG_TYPE -u RELEEM_MYSQL_HOST -u RELEEM_MYSQL_LOGIN -u RELEEM_MYSQL_PASSWORD -u RELEEM_MYSQL_ROOT_LOGIN -u RELEEM_MYSQL_ROOT_PASSWORD -u RELEEM_MYSQL_TYPE \
        RELEEM_TEST_MODE=1 \
        RELEEM_WORKDIR="${workdir}" \
        RELEEM_CONF_FILE="${conf}" \
        MYSQL_ARGS_LOG="${mysql_args}" \
        MYSQL_SQL_LOG="${mysql_sql}" \
        RELEEM_CRON_ENABLE="1" \
        bash "${MOCK_BIN}/enable_query_optimization" enable_query_optimization

    [ "$status" -eq 0 ]
    run grep -F -- "HEX(User)='6F277265696C6C79'" "${mysql_args}"
    [ "$status" -eq 0 ]
    run grep -F -- "REPLACE(User, CHAR(96)" "${mysql_args}"
    [ "$status" -eq 0 ]
    run grep -F -- "REPLACE(Host, CHAR(96)" "${mysql_args}"
    [ "$status" -eq 0 ]
    run grep -F -- "where User='releem'" "${mysql_args}"
    [ "$status" -ne 0 ]
    run assert_exact_sql_payloads "${mysql_sql}" \
        "${account_catalog_sql}" \
        "GRANT SELECT on *.* to \`o'reilly\`@\`localhost\`;"
    [ "$status" -eq 0 ]
    run grep -E "^query_optimization=true$" "${conf}"
    [ "$status" -eq 0 ]
}

@test "SQL payload assertions reject extra statements and other principals" {
    local sql_log="${TEST_TMPDIR}/database.sql"
    local expected="GRANT SELECT on *.* to \`collector\`@\`localhost\`;"
    local payload
    local -a invalid_payloads=(
        "${expected} ALTER USER \`collector\`@\`localhost\` IDENTIFIED BY 'changed';"
        "${expected} DROP USER \`other\`@\`localhost\`;"
        "${expected} GRANT PROCESS ON *.* TO \`collector\`@\`localhost\`;"
        "GRANT SELECT on *.* to \`other\`@\`localhost\`;"
    )

    for payload in "${invalid_payloads[@]}"; do
        printf '%s\0' "${payload}" > "${sql_log}"
        run assert_exact_sql_payloads "${sql_log}" "${expected}"
        [ "$status" -ne 0 ]
    done

    local -a separate_unsafe_payloads=(
        "ALTER USER \`collector\`@\`localhost\` IDENTIFIED BY 'changed';"
        "CALL rotate_monitoring_password();"
        "SET PASSWORD FOR \`collector\`@\`localhost\` = 'hash';"
    )
    for payload in "${separate_unsafe_payloads[@]}"; do
        printf '%s\0' "${expected}" "${payload}" > "${sql_log}"
        run assert_exact_sql_payloads "${sql_log}" "${expected}"
        [ "$status" -ne 0 ]
    done
}

@test "load_runtime_config preserves existing query optimization setting" {
    local workdir="${TEST_TMPDIR}/workdir"
    mkdir -p "${workdir}"
    printf 'apikey="k1"\nquery_optimization=false # existing setting\n' > "${workdir}/releem.conf"

    run bash -c '
        RELEEM_TEST_MODE=1 RELEEM_WORKDIR="$1" source "$2"
        unset query_optimization RELEEM_QUERY_OPTIMIZATION
        load_runtime_config
        printf "%s|%s" "$RELEEM_QUERY_OPTIMIZATION" "$query_optimization"
    ' _ "${workdir}" "${INSTALL_SH}"

    [ "$status" -eq 0 ]
    [ "$output" = "false|false" ]
}

@test "enable_query_optimization fails when configured MySQL account is missing" {
    prepare_common_install_mocks
    create_mock_cmd "mysqladmin" 'echo "mysqld is alive"'
    create_mock_cmd "mariadb-admin" 'echo "mysqld is alive"'
    create_mock_cmd "mysql" '
printf "%s\n" "$*" >> "${MYSQL_ARGS_LOG}"
exit 0
'
    create_mock_cmd "mariadb" '
printf "%s\n" "$*" >> "${MYSQL_ARGS_LOG}"
exit 0
'

    local workdir="${TEST_TMPDIR}/workdir"
    local conf="${workdir}/releem.conf"
    local mysql_args="${TEST_TMPDIR}/mysql.args"
    mkdir -p "${workdir}"
    printf 'apikey="k1"\nmysql_user="missing"\nmysql_password="secret"\nmysql_host="127.0.0.1"\nmysql_port="3306"\n' > "${conf}"
    printf '#!/usr/bin/env bash\nexit 0\n' > "${workdir}/releem-agent"
    printf '#!/usr/bin/env bash\nexit 0\n' > "${workdir}/releem.sh"
    chmod +x "${workdir}/releem-agent" "${workdir}/releem.sh"
    cp "${INSTALL_SH}" "${MOCK_BIN}/enable_query_optimization"
    chmod +x "${MOCK_BIN}/enable_query_optimization"

    PATH="${MOCK_BIN}:${PATH}" run env -u RELEEM_MYSQL_LOGIN -u RELEEM_MYSQL_PASSWORD \
        RELEEM_TEST_MODE=1 \
        RELEEM_WORKDIR="${workdir}" \
        RELEEM_CONF_FILE="${conf}" \
        MYSQL_ARGS_LOG="${mysql_args}" \
        RELEEM_CRON_ENABLE="1" \
        bash "${MOCK_BIN}/enable_query_optimization" enable_query_optimization

    [ "$status" -ne 0 ]
    run grep -E "^query_optimization=true$" "${conf}"
    [ "$status" -ne 0 ]
}

@test "enable_query_optimization mode grants pg_read_all_data for postgresql" {
    prepare_common_install_mocks
    create_mock_cmd "psql" '
printf "%s\n" "$*" >> "${PG_ARGS_LOG}"
if [[ "$*" == *"SHOW server_version_num"* ]]; then
  echo "140000"
elif [[ "$*" == *"SELECT datname"* && "$*" == *"pg_database"* ]]; then
  printf "postgres\000shop\000"
fi
exit 0
'

    local workdir="${TEST_TMPDIR}/workdir"
    local conf="${workdir}/releem.conf"
    local pg_args="${TEST_TMPDIR}/pg.args"
    mkdir -p "${workdir}"
    printf 'apikey="k1"\n' > "${conf}"
    printf '#!/usr/bin/env bash\nexit 0\n' > "${workdir}/releem-agent"
    printf '#!/usr/bin/env bash\necho "$@" >> "%s/calls.log"\nexit 0\n' "${workdir}" > "${workdir}/releem.sh"
    chmod +x "${workdir}/releem-agent" "${workdir}/releem.sh"
    cp "${INSTALL_SH}" "${MOCK_BIN}/enable_query_optimization"
    chmod +x "${MOCK_BIN}/enable_query_optimization"

    PATH="${MOCK_BIN}:${PATH}" run env \
        PATH="${MOCK_BIN}:${PATH}" \
        RELEEM_TEST_MODE=1 \
        RELEEM_WORKDIR="${workdir}" \
        RELEEM_CONF_FILE="${conf}" \
        RELEEM_API_KEY="k1" \
        RELEEM_PG_ROOT_LOGIN="pgadmin" \
        RELEEM_PG_ROOT_PASSWORD="rootpwd" \
        PG_ARGS_LOG="${pg_args}" \
        RELEEM_CRON_ENABLE="1" \
        bash "${MOCK_BIN}/enable_query_optimization" enable_query_optimization

    [ "$status" -eq 0 ]
    run grep -F -- 'GRANT CONNECT ON DATABASE "postgres" TO "releem";' "${pg_args}"
    [ "$status" -eq 0 ]
    run grep -F -- 'GRANT CONNECT ON DATABASE "shop" TO "releem";' "${pg_args}"
    [ "$status" -eq 0 ]
    run grep -F -- 'GRANT pg_read_all_data TO "releem";' "${pg_args}"
    [ "$status" -eq 0 ]
    for procedural_sql in 'DO $$' 'EXECUTE format' 'set_config' 'decode(' 'ALTER DEFAULT PRIVILEGES' 'secret' 'pg_password'; do
        run grep -F -- "${procedural_sql}" "${pg_args}"
        [ "$status" -ne 0 ]
    done
    run grep -E "^query_optimization=true$" "${conf}"
    [ "$status" -eq 0 ]
    run grep -E "^-p$" "${workdir}/calls.log"
    [ "$status" -eq 0 ]
}



@test "fresh download installs both scripts and calls releem even when a legacy script existed" {
    create_mock_cmd "sudo" '"$@"'
    create_mock_cmd "curl" '
printf "%s\n" "$*" >> "${DOWNLOAD_LOG}"
while [ "$#" -gt 0 ]; do
    if [ "$1" = -o ]; then out="$2"; shift 2; else shift; fi
done
if [[ "$out" == *releem-agent* ]]; then
    cp /bin/true "$out"
else
    printf '\''#!/usr/bin/env bash\nprintf "%%s:%%s\\n" "${0##*/}" "$*" >> "$CONFIGURER_CALLS"\n'\'' > "$out"
fi
'
    local workdir="${TEST_TMPDIR}/workdir"
    mkdir -p "$workdir"
    printf '#!/usr/bin/env bash\nexit 91\n' > "$workdir/mysqlconfigurer.sh"
    run env PATH="${MOCK_BIN}:${PATH}" DOWNLOAD_LOG="${TEST_TMPDIR}/download.log" CONFIGURER_CALLS="${TEST_TMPDIR}/calls" \
        RELEEM_TEST_MODE=1 RELEEM_WORKDIR="$workdir" bash -c 'source "$1"; download_releem_agent; $RELEEM_COMMAND -p' _ "$INSTALL_SH"
    [ "$status" -eq 0 ]
    [ -x "$workdir/releem.sh" ]
    [ -x "$workdir/mysqlconfigurer.sh" ]
    [ "$(wc -l < "${TEST_TMPDIR}/download.log")" -eq 3 ]
    grep -F mysqlconfigurer.sh "${TEST_TMPDIR}/download.log"
    [ "$(cat "${TEST_TMPDIR}/calls")" = releem.sh:-p ]
}

@test "fresh install reinstall and update stage root v2 artifacts inside the workdir" {
    prepare_common_install_mocks
    create_mock_cmd "curl" '
out=""; url=""; fail_http=0
while [ "$#" -gt 0 ]; do
    case "$1" in
        --fail) fail_http=1; shift ;;
        -o) out="$2"; shift 2 ;;
        https://*) url="$1"; shift ;;
        *) shift ;;
    esac
done
case "$out" in "$RELEEM_WORKDIR"/.releem-release.*/"${out##*/}") ;; *) exit 95 ;; esac
[ -d "${out%/*}" ] || exit 96
printf "%s\n" "${out%/*}" >> "$STAGING_LOG"
printf "%s\n" "$url" >> "$DOWNLOAD_LOG"
[ "$fail_http" -eq 1 ] || exit 94
case "$url" in
    https://releem.s3.amazonaws.com/v2/releem-agent-x86_64) cp /bin/true "$out" ;;
    https://releem.s3.amazonaws.com/v2/releem.sh|https://releem.s3.amazonaws.com/v2/mysqlconfigurer.sh)
        printf "#!/bin/sh\nexit 0\n" > "$out" ;;
    *) exit 93 ;;
esac
'
    local scenario action workdir="${TEST_TMPDIR}/workdir" downloads="${TEST_TMPDIR}/downloads"
    for scenario in fresh reinstall update; do
        rm -rf "$workdir"
        action=download_releem_agent
        if [ "$scenario" != fresh ]; then
            mkdir -p "$workdir/conf" "$workdir/.old-dir"
            cp /bin/true "$workdir/releem-agent"
            echo old > "$workdir/releem.conf"
            echo old > "$workdir/conf/old.cnf"
            echo old > "$workdir/.old-dir/file"
        fi
        [ "$scenario" != update ] || action=releem_update
        : > "$downloads"
        : > "${TEST_TMPDIR}/staging"
        run env PATH="${MOCK_BIN}:${PATH}" RELEEM_TEST_MODE=1 RELEEM_WORKDIR="$workdir" \
            TMPDIR="$TEST_TMPDIR" RELEEM_API_KEY=k1 DOWNLOAD_LOG="$downloads" STAGING_LOG="${TEST_TMPDIR}/staging" \
            bash -c 'source "$1"; "$2"' _ "$INSTALL_SH" "$action"
        [ "$status" -eq 0 ]
        [ "$(sort -u "${TEST_TMPDIR}/staging" | wc -l)" -eq 1 ]
        [ -z "$(find "$workdir" -name '.releem-release.*' -print)" ]
        [ -x "$workdir/releem-agent" ]
        [ -x "$workdir/releem.sh" ]
        [ -x "$workdir/mysqlconfigurer.sh" ]
        if [ "$scenario" = update ]; then
            [ -f "$workdir/releem.conf" ]
            [ -f "$workdir/conf/old.cnf" ]
            [ -f "$workdir/.old-dir/file" ]
        else
            [ -d "$workdir/conf" ]
            [ ! -e "$workdir/releem.conf" ]
            [ ! -e "$workdir/conf/old.cnf" ]
            [ ! -e "$workdir/.old-dir" ]
        fi
        [ "$(cat "$downloads")" = "$(printf 'https://releem.s3.amazonaws.com/v2/%s\n' \
            releem-agent-x86_64 releem.sh mysqlconfigurer.sh)" ]
    done
}

prepare_release_failure_mocks() {
    prepare_common_install_mocks
    create_mock_cmd "curl" '
out=""; url=""; fail_http=0
while [ "$#" -gt 0 ]; do
    case "$1" in
        --fail) fail_http=1; shift ;;
        -o) out="$2"; shift 2 ;;
        https://*) url="$1"; shift ;;
        *) shift ;;
    esac
done
case "$url" in
    https://releem.s3.amazonaws.com/v2/releem-agent-x86_64|https://releem.s3.amazonaws.com/v2/releem.sh|https://releem.s3.amazonaws.com/v2/mysqlconfigurer.sh) ;;
    *) exit 93 ;;
esac
if [[ "$url" == */"$FAIL_ARTIFACT" ]]; then
    case "$FAIL_KIND" in
        http) [ "$fail_http" = 0 ] || exit 22; echo "<html>not found</html>" > "$out" ;;
        empty) : > "$out" ;;
        missing) rm -f "$out" ;;
        syntax) printf "if then\n" > "$out" ;;
        corrupt) printf '\''#!/bin/sh\necho executed > "$BINARY_EXECUTED"\n'\'' > "$out" ;;
    esac
elif [[ "$url" == */releem-agent-* ]]; then
    cp /bin/true "$out"
else
    printf "#!/bin/sh\nexit 0\n" > "$out"
fi
'
}

@test "release download failures preserve update files and leave reinstalls uninstalled" {
    prepare_release_failure_mocks
    local workdir="${TEST_TMPDIR}/workdir" original="${TEST_TMPDIR}/original"
    mkdir -p "$workdir/conf" "$original"
    printf '#!/bin/sh\necho "$*" >> "$SERVICE_CALLS"\nrm -f "$SERVICE_RUNNING"\n' > "$workdir/releem-agent"
    printf '#!/bin/sh\necho original\n' > "$workdir/releem.sh"
    cp "$workdir/releem.sh" "$workdir/mysqlconfigurer.sh"
    echo original-config > "$workdir/releem.conf"
    echo original-conf > "$workdir/conf/database.cnf"
    chmod +x "$workdir/releem-agent" "$workdir/releem.sh" "$workdir/mysqlconfigurer.sh"
    cp -R "$workdir/." "$original/"
    touch "${TEST_TMPDIR}/running"
    local action artifact failure
    for action in download_releem_agent releem_update; do
        for artifact in releem-agent-x86_64 releem.sh mysqlconfigurer.sh; do
            for failure in http empty missing syntax corrupt; do
                [ "$artifact:$failure" != releem-agent-x86_64:syntax ] || continue
                [ "$failure" != corrupt ] || [ "$artifact" = releem-agent-x86_64 ] || continue
                rm -rf "$workdir"
                mkdir -p "$workdir"
                cp -R "$original/." "$workdir/"
                run env PATH="${MOCK_BIN}:${PATH}" RELEEM_TEST_MODE=1 RELEEM_WORKDIR="$workdir" \
                    TMPDIR="$TEST_TMPDIR" RELEEM_API_KEY=k1 FAIL_ARTIFACT="$artifact" FAIL_KIND="$failure" \
                    SERVICE_CALLS="${TEST_TMPDIR}/service.calls" SERVICE_RUNNING="${TEST_TMPDIR}/running" \
                    BINARY_EXECUTED="${TEST_TMPDIR}/binary.executed" \
                    bash -c 'source "$1"; "$2"' _ "$INSTALL_SH" "$action"
                if [ "$status" -ne 1 ]; then
                    echo "unexpected status $status: $action $artifact $failure: $output"
                    return 1
                fi
                if [ "$action" = releem_update ]; then
                    diff -r "$original" "$workdir"
                else
                    [ -d "$workdir/conf" ]
                    [ -z "$(find "$workdir" -mindepth 1 ! -path "$workdir/conf" -print)" ]
                fi
                [ ! -e "${TEST_TMPDIR}/service.calls" ]
                [ -e "${TEST_TMPDIR}/running" ]
                [ ! -e "${TEST_TMPDIR}/binary.executed" ]
            done
        done
    done
}

@test "fresh release failures clean staging and leave an empty configuration directory" {
    prepare_release_failure_mocks
    local existing failure workdir="${TEST_TMPDIR}/new-install"
    for existing in no yes; do
        for failure in http empty missing syntax; do
            rm -rf "$workdir"
            [ "$existing" != yes ] || mkdir -p "$workdir"
            run env PATH="${MOCK_BIN}:${PATH}" RELEEM_TEST_MODE=1 RELEEM_WORKDIR="$workdir" \
                TMPDIR="$TEST_TMPDIR" FAIL_ARTIFACT=mysqlconfigurer.sh FAIL_KIND="$failure" \
                bash -c 'source "$1"; download_releem_agent' _ "$INSTALL_SH"
            [ "$status" -eq 1 ]
            [ -d "$workdir/conf" ]
            [ -z "$(find "$workdir" -mindepth 1 ! -path "$workdir/conf" -print)" ]
        done
    done
}

@test "staging creation failure leaves an empty configuration directory" {
    prepare_common_install_mocks
    create_mock_cmd "mktemp" 'printf "%s\n" "$*" > "$MKTEMP_CALLS"; exit 87'
    local existing workdir="${TEST_TMPDIR}/new-install"
    for existing in no yes; do
        rm -rf "$workdir"
        rm -f "${TEST_TMPDIR}/mktemp.calls"
        [ "$existing" != yes ] || mkdir -p "$workdir"
        run env PATH="${MOCK_BIN}:${PATH}" RELEEM_TEST_MODE=1 RELEEM_WORKDIR="$workdir" \
            MKTEMP_CALLS="${TEST_TMPDIR}/mktemp.calls" \
            bash -c 'source "$1"; download_releem_agent' _ "$INSTALL_SH"
        [ "$status" -eq 1 ]
        [ "$(cat "${TEST_TMPDIR}/mktemp.calls")" = "-d $workdir/.releem-release.XXXXXX" ]
        [ -d "$workdir/conf" ]
        [ -z "$(find "$workdir" -mindepth 1 ! -path "$workdir/conf" -print)" ]
    done
}

@test "cron setup uses neutral name and preserves existing legacy jobs" {
    create_mock_cmd "sudo" '"$@"'
    create_mock_cmd "crontab" '
if [ "$1" = -l ]; then cat "${CRON_FILE}"; else cat > "${CRON_FILE}.new"; mv "${CRON_FILE}.new" "${CRON_FILE}"; fi
'
    local cronfile="${TEST_TMPDIR}/cron"
    printf '15 3 * * * unrelated-command\n' > "$cronfile"
    run env PATH="${MOCK_BIN}:${PATH}" CRON_FILE="$cronfile" RELEEM_TEST_MODE=1 RELEEM_CRON_ENABLE=1 \
        RELEEM_WORKDIR="${TEST_TMPDIR}/workdir" bash -c 'source "$1"; configure_crontab; configure_crontab' _ "$INSTALL_SH"
    [ "$status" -eq 0 ]
    [ "$(grep -c releem.sh "$cronfile")" -eq 1 ]
    [ "$(grep -c unrelated-command "$cronfile")" -eq 1 ]
    printf '15 3 * * * unrelated-command\n17 4 * * * /bin/bash %s/mysqlconfigurer.sh -u\n' "${TEST_TMPDIR}/workdir" > "$cronfile"
    cp "$cronfile" "${cronfile}.original"
    run env PATH="${MOCK_BIN}:${PATH}" CRON_FILE="$cronfile" RELEEM_TEST_MODE=1 RELEEM_CRON_ENABLE=1 \
        RELEEM_WORKDIR="${TEST_TMPDIR}/workdir" bash -c 'source "$1"; configure_crontab' _ "$INSTALL_SH"
    [ "$status" -eq 0 ]
    cmp "$cronfile" "${cronfile}.original"
    [[ "$output" == *"Automatic updates are enabled."* ]]
}

@test "cron setup installs an active job when legacy or neutral entries are commented out" {
    create_mock_cmd "sudo" '"$@"'
    create_mock_cmd "crontab" '
if [ "$1" = -l ]; then cat "${CRON_FILE}"; else cat > "${CRON_FILE}.new"; mv "${CRON_FILE}.new" "${CRON_FILE}"; fi
'
    local cronfile="${TEST_TMPDIR}/cron"
    local workdir="${TEST_TMPDIR}/workdir"
    local script
    for script in mysqlconfigurer.sh releem.sh; do
        printf '#17 4 * * * /bin/bash %s/%s -u\n \t#18 5 * * * /bin/bash %s/%s -u\n15 3 * * * unrelated-command\n' \
            "$workdir" "$script" "$workdir" "$script" > "$cronfile"
        cp "$cronfile" "${cronfile}.expected"
        printf '00 00 * * * PATH=/bin:/sbin:/usr/bin:/usr/sbin /bin/bash %s/releem.sh -u\n' "$workdir" >> "${cronfile}.expected"
        run env PATH="${MOCK_BIN}:${PATH}" CRON_FILE="$cronfile" RELEEM_TEST_MODE=1 RELEEM_CRON_ENABLE=1 \
            RELEEM_WORKDIR="$workdir" bash -c 'source "$1"; configure_crontab; configure_crontab' _ "$INSTALL_SH"
        [ "$status" -eq 0 ]
        cmp "$cronfile" "${cronfile}.expected"
        [[ "$output" == *"Automatic updates are enabled."* ]]
    done
}

@test "cron verification reports disabled when only commented jobs remain after a write" {
    create_mock_cmd "sudo" '"$@"'
    create_mock_cmd "crontab" '
if [ "$1" = -l ]; then cat "${CRON_FILE}"; else cat >/dev/null; fi
'
    local cronfile="${TEST_TMPDIR}/cron"
    local workdir="${TEST_TMPDIR}/workdir"
    printf '#17 4 * * * /bin/bash %s/mysqlconfigurer.sh -u\n \t#18 5 * * * /bin/bash %s/releem.sh -u\n' \
        "$workdir" "$workdir" > "$cronfile"
    run env PATH="${MOCK_BIN}:${PATH}" CRON_FILE="$cronfile" RELEEM_TEST_MODE=1 RELEEM_CRON_ENABLE=1 \
        RELEEM_WORKDIR="$workdir" bash -c 'source "$1"; configure_crontab' _ "$INSTALL_SH"
    [ "$status" -eq 0 ]
    [[ "$output" == *"Crontab configuration failed. Automatic updates are disabled."* ]]
    [[ "$output" != *"Automatic updates are enabled."* ]]
}

@test "update mode downloads artifacts and restarts agent with mocked side-effects" {
    prepare_common_install_mocks
    create_mock_cmd "curl" '
out=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) out="$2"; shift 2 ;;
    *) shift ;;
  esac
done
echo "$out" >> "${UPDATE_CURL_LOG}"
if [ -n "$out" ]; then
  mkdir -p "$(dirname "$out")"
  if [[ "$out" == *releem-agent* ]]; then
    cp /bin/true "$out"
  else
  cat >"$out" <<'"'"'INNER'"'"'
#!/usr/bin/env bash
echo "$@" >> "${UPDATE_AGENT_LOG}"
exit 0
INNER
  chmod +x "$out"
  fi
fi
echo "200"
'

    create_mock_cmd "crontab" 'echo called >> "${UPDATE_CRON_LOG}"; exit 99'

    local workdir="${TEST_TMPDIR}/workdir"
    local curl_calls="${TEST_TMPDIR}/curl.calls"
    local agent_calls="${TEST_TMPDIR}/agent.calls"
    mkdir -p "${workdir}"
    cat > "${workdir}/releem-agent" <<'EOF'
#!/usr/bin/env bash
echo "$@" >> "${UPDATE_AGENT_LOG}"
exit 0
EOF
    cat > "${workdir}/mysqlconfigurer.sh" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
    chmod +x "${workdir}/releem-agent" "${workdir}/mysqlconfigurer.sh"

    PATH="${MOCK_BIN}:${PATH}" run env \
        RELEEM_TEST_MODE=1 \
        RELEEM_WORKDIR="${workdir}" \
        RELEEM_API_KEY="k1" \
        UPDATE_CRON_LOG="${TEST_TMPDIR}/cron.calls" \
        UPDATE_CURL_LOG="${curl_calls}" \
        UPDATE_AGENT_LOG="${agent_calls}" \
        bash "${INSTALL_SH}" update

    [ "$status" -eq 0 ]
    [ -x "${workdir}/releem.sh" ]
    [ -x "${workdir}/mysqlconfigurer.sh" ]
    [ ! -e "${TEST_TMPDIR}/cron.calls" ]
    [ "$(wc -l < "${curl_calls}")" -eq 3 ]
    run grep -F "releem.sh.new" "${curl_calls}"
    [ "$status" -eq 0 ]
    run grep -E "stop|start|-f" "${agent_calls}"
    [ "$status" -eq 0 ]
}

@test "update succeeds with a warning when post-update configuration generation fails" {
    prepare_common_install_mocks
    local workdir="${TEST_TMPDIR}/workdir" replacement="${TEST_TMPDIR}/replacement-agent"
    mkdir -p "$workdir"
    cat > "$replacement" <<'EOF'
#!/bin/sh
printf '%s\n' "$*" >> "$AGENT_CALLS"
if [ "$1" = -f ]; then
    echo 'configuration API unavailable' >&2
    exit 1
fi
EOF
    cp "$replacement" "$workdir/releem-agent"
    chmod +x "$replacement" "$workdir/releem-agent"
    run env PATH="${MOCK_BIN}:${PATH}" RELEEM_TEST_MODE=1 RELEEM_WORKDIR="$workdir" \
        RELEEM_API_KEY=k1 REPLACEMENT_AGENT="$replacement" AGENT_CALLS="${TEST_TMPDIR}/agent.calls" \
        bash -c '
            source "$1"
            download_release_artifacts() {
                cp "$REPLACEMENT_AGENT" "$1/releem-agent.new"
                printf "#!/bin/sh\n# installed script\n" > "$1/releem.sh.new"
                cp "$1/releem.sh.new" "$1/mysqlconfigurer.sh.new"
                chmod 755 "$1/"*.new
            }
            releem_update
        ' _ "$INSTALL_SH"
    [ "$status" -eq 0 ]
    [[ "$output" == *"WARNING:"* ]]
    [[ "$output" == *"Releem Agent updated successfully."* ]]
    [ "$(cat "${TEST_TMPDIR}/agent.calls")" = "$(printf '%s\n' stop start -f)" ]
    cmp "$replacement" "$workdir/releem-agent"
    grep -q 'installed script' "$workdir/releem.sh"
    [ -z "$(find "$workdir" -name '.releem-release.*' -print)" ]
}

@test "reinstall and update validate unreadable downloads through sudo under restrictive umask" {
    prepare_common_install_mocks
    create_mock_cmd "sudo" '
if [ "$1" = curl ]; then
  "$@" || exit $?
  # Model root-owned downloads that the installer user cannot read.
  for arg in "$@"; do
    case "$arg" in *.new) chmod 000 "$arg" ;; esac
  done
elif [ "$1" = bash ] && [ "$2" = -n ]; then
  chmod u+r "$3"
  "$@"
  result=$?
  chmod 000 "$3"
  basename "$3" >> "${UPDATE_SYNTAX_LOG}"
  exit "$result"
elif [ "$1" = test ]; then
  case "$2" in
    -f|-s) printf "%s %s\n" "$2" "${3##*/}" >> "${UPDATE_VALIDATION_LOG}" ;;
  esac
  "$@"
elif [ "$1" = od ]; then
  file="${@: -1}"
  chmod u+r "$file"
  "$@"
  result=$?
  chmod 000 "$file"
  echo od >> "${UPDATE_BINARY_CHECK_LOG}"
  exit "$result"
else
  "$@"
fi
'
    local workdir="${TEST_TMPDIR}/workdir"
    local checks="${TEST_TMPDIR}/syntax.calls"
    mkdir -p "$workdir"
    printf '#!/bin/sh\nexit 0\n' > "$workdir/releem-agent"
    chmod +x "$workdir/releem-agent"

    local action
    for action in download_releem_agent releem_update; do
        rm -f "$checks" "${TEST_TMPDIR}/validation.calls" "${TEST_TMPDIR}/binary-check.calls"
        run env PATH="${MOCK_BIN}:${PATH}" RELEEM_TEST_MODE=1 TMPDIR="$TEST_TMPDIR" \
            RELEEM_WORKDIR="$workdir" RELEEM_API_KEY="k1" UPDATE_SYNTAX_LOG="$checks" \
            UPDATE_VALIDATION_LOG="${TEST_TMPDIR}/validation.calls" \
            UPDATE_BINARY_CHECK_LOG="${TEST_TMPDIR}/binary-check.calls" \
            bash -c 'umask 077; source "$1"; sudo_cmd=sudo; "$2"' _ "$INSTALL_SH" "$action"

        [ "$status" -eq 0 ]
        [ -x "$workdir/releem-agent" ]
        [ -x "$workdir/releem.sh" ]
        [ -x "$workdir/mysqlconfigurer.sh" ]
        [ "$(cat "${TEST_TMPDIR}/binary-check.calls")" = od ]
        # Verify privilege use even when the test suite itself runs as root.
        [ "$(cat "$checks")" = "$(printf '%s\n%s' releem.sh.new mysqlconfigurer.sh.new)" ]
        [ "$(cat "${TEST_TMPDIR}/validation.calls")" = "$(printf '%s\n' \
            '-f releem-agent.new' '-s releem-agent.new' '-f releem.sh.new' '-s releem.sh.new' \
            '-f mysqlconfigurer.sh.new' '-s mysqlconfigurer.sh.new')" ]
    done
}

@test "postgresql catalog grants preserve newline identifiers from NUL records" {
    create_mock_cmd "psql" '
query="$*"
printf "%s\n" "$query" >> "${PG_ARGS_LOG}"
if [[ "$query" == *"SHOW server_version_num"* ]]; then
  printf "120000\n"
elif [[ "$query" == *"SELECT datname"* && "$query" == *"pg_database"* ]]; then
  [[ " ${query} " == *" -0 "* ]] || exit 91
  printf "catalog\ndb\000plain db\000"
elif [[ "$query" == *"SELECT nspname"* && "$query" == *"pg_namespace"* ]]; then
  [[ " ${query} " == *" -0 "* ]] || exit 92
  printf "sales\nschema\000odd \"schema\"\000"
elif [[ "$query" == *"GRANT"* ]]; then
  [[ "$query" == *"-v ON_ERROR_STOP=1"* ]] || exit 93
  cat >/dev/null
fi
exit 0
'

    run env \
        RELEEM_PG_ROOT_PASSWORD="rootpwd" \
        PG_ARGS_LOG="${TEST_TMPDIR}/pg.args" \
        bash -c '
            RELEEM_TEST_MODE=1 source "$1"
            psqlcmd="$2"
            pg_root_peer_connection=""
            pg_root_connection_string="-h 127.0.0.1 -p 5432 -d postgres"
            grant_postgresql_query_optimization_access "postgres" '"'"'role"reader'"'"'
        ' _ "${INSTALL_SH}" "${MOCK_BIN}/psql"

    [ "$status" -eq 0 ]
    run grep -F -- $'GRANT CONNECT ON DATABASE "catalog\ndb" TO "role""reader";' "${TEST_TMPDIR}/pg.args"
    [ "$status" -eq 0 ]
    run grep -F -- 'GRANT CONNECT ON DATABASE "plain db" TO "role""reader";' "${TEST_TMPDIR}/pg.args"
    [ "$status" -eq 0 ]
    run grep -F -- $'-d catalog\ndb -v ON_ERROR_STOP=1 -c GRANT USAGE ON SCHEMA "sales\nschema" TO "role""reader";' "${TEST_TMPDIR}/pg.args"
    [ "$status" -eq 0 ]
    run grep -F -- 'GRANT SELECT ON ALL TABLES IN SCHEMA "odd ""schema""" TO "role""reader";' "${TEST_TMPDIR}/pg.args"
    [ "$status" -eq 0 ]
}

@test "postgresql catalog grants do not require mapfile delimiter support" {
    run bash -c '
        RELEEM_TEST_MODE=1 source "$1"
        mapfile() { return 97; }
        postgresql_root_exec() {
            case "$*" in
                *"SHOW server_version_num"*) printf "140000\n" ;;
                *"pg_database"*) printf "postgres\000" ;;
                *"pg_namespace"*) printf "app\000" ;;
                *"GRANT CONNECT"*) printf "%s\n" "$*" >&2 ;;
                *"GRANT pg_read_all_data"*) printf "%s\n" "$*" >&2 ;;
                *"ALTER ROLE"*) printf "%s\n" "$*" >&2 ;;
            esac
        }
        grant_postgresql_query_optimization_access postgres releem
    ' _ "${INSTALL_SH}"

    [ "$status" -eq 0 ]
    [[ "$output" == *'GRANT CONNECT ON DATABASE "postgres" TO "releem";'* ]]
    [[ "$output" == *'GRANT pg_read_all_data TO "releem";'* ]]
    [[ "$output" == *'ALTER ROLE "releem" IN DATABASE "postgres" SET search_path TO "$user", "public", "app";'* ]]
}

@test "postgresql query optimization sets per-database search_path for pg14" {
    run bash -c '
        RELEEM_TEST_MODE=1 source "$1"
        postgresql_root_exec() {
            printf "%s\n" "$*" >&2
            case "$*" in
                *"SHOW server_version_num"*) printf "140000\n" ;;
                *"pg_database"*) printf "appdb\000" ;;
                *"pg_namespace"*) printf "app\000analytics\000" ;;
                *"GRANT CONNECT"*|*"GRANT pg_read_all_data"*|*"ALTER ROLE"*) ;;
            esac
        }
        grant_postgresql_query_optimization_access postgres releem
    ' _ "${INSTALL_SH}"

    [ "$status" -eq 0 ]
    [[ "$output" == *'GRANT pg_read_all_data TO "releem";'* ]]
    [[ "$output" == *'ALTER ROLE "releem" IN DATABASE "appdb" SET search_path TO "$user", "public", "app", "analytics";'* ]]
}

@test "postgresql query optimization sets search_path with only defaults when no user schemas" {
    run bash -c '
        RELEEM_TEST_MODE=1 source "$1"
        postgresql_root_exec() {
            printf "%s\n" "$*" >&2
            case "$*" in
                *"SHOW server_version_num"*) printf "140000\n" ;;
                *"pg_database"*) printf "postgres\000" ;;
                *"pg_namespace"*) printf "" ;;
                *"GRANT"*|*"ALTER ROLE"*) ;;
            esac
        }
        grant_postgresql_query_optimization_access postgres releem
    ' _ "${INSTALL_SH}"

    [ "$status" -eq 0 ]
    [[ "$output" == *'ALTER ROLE "releem" IN DATABASE "postgres" SET search_path TO "$user", "public";'* ]]
}

@test "postgresql query optimization sets search_path for pg12 after schema grants" {
    run bash -c '
        RELEEM_TEST_MODE=1 source "$1"
        postgresql_root_exec() {
            printf "%s\n" "$*" >&2
            case "$*" in
                *"SHOW server_version_num"*) printf "120000\n" ;;
                *"pg_database"*) printf "shop\000" ;;
                *"pg_namespace"*) printf "sales\000" ;;
                *"GRANT"*|*"ALTER ROLE"*) ;;
            esac
        }
        grant_postgresql_query_optimization_access postgres releem
    ' _ "${INSTALL_SH}"

    [ "$status" -eq 0 ]
    [[ "$output" == *'GRANT USAGE ON SCHEMA "sales" TO "releem";'* ]]
    [[ "$output" == *'ALTER ROLE "releem" IN DATABASE "shop" SET search_path TO "$user", "public", "sales";'* ]]
}

@test "postgresql database catalog failure stops before grants" {
    create_mock_cmd "psql" '
query="$*"
printf "%s\n" "$query" >> "${PG_ARGS_LOG}"
if [[ "$query" == *"SHOW server_version_num"* ]]; then
  printf "140000\n"
elif [[ "$query" == *"SELECT datname"* && "$query" == *"pg_database"* ]]; then
  exit 41
fi
exit 0
'

    run env PG_ARGS_LOG="${TEST_TMPDIR}/pg.args" bash -c '
        RELEEM_TEST_MODE=1 source "$1"
        psqlcmd="$2"
        pg_root_peer_connection=""
        pg_root_connection_string="-d postgres"
        grant_postgresql_query_optimization_access postgres releem
    ' _ "${INSTALL_SH}" "${MOCK_BIN}/psql"

    [ "$status" -ne 0 ]
    run grep -F -- "GRANT" "${TEST_TMPDIR}/pg.args"
    [ "$status" -ne 0 ]
}

@test "postgresql schema catalog failure stops later grants" {
    run bash -c '
        RELEEM_TEST_MODE=1 source "$1"
        postgresql_root_exec() {
            printf "%s\n" "$*" >&2
            case "$*" in
                *"SHOW server_version_num"*) printf "120000\n" ;;
                *"pg_database"*) printf "first\000second\000" ;;
                *"pg_namespace"*) return 42 ;;
            esac
        }
        grant_postgresql_query_optimization_access postgres releem
    ' _ "${INSTALL_SH}"

    [ "$status" -ne 0 ]
    [[ "$output" != *"GRANT USAGE ON SCHEMA"* ]]
    [[ "$output" != *"GRANT SELECT ON ALL TABLES"* ]]
}

@test "postgresql grant failure stops later grants" {
    run bash -c '
        RELEEM_TEST_MODE=1 source "$1"
        postgresql_root_exec() {
            printf "%s\n" "$*" >&2
            case "$*" in
                *"SHOW server_version_num"*) printf "140000\n" ;;
                *"pg_database"*) printf "first\000second\000" ;;
                *"GRANT CONNECT ON DATABASE \"first\""*)
                    [[ "$*" == *"-v ON_ERROR_STOP=1"* ]] || return 93
                    return 43
                    ;;
                *"GRANT"*) [[ "$*" == *"-v ON_ERROR_STOP=1"* ]] || return 93 ;;
            esac
        }
        grant_postgresql_query_optimization_access postgres releem
    ' _ "${INSTALL_SH}"

    [ "$status" -ne 0 ]
    [[ "$output" != *'GRANT CONNECT ON DATABASE "second"'* ]]
    [[ "$output" != *'GRANT pg_read_all_data'* ]]
}

@test "postgresql alter role failure propagates" {
    run bash -c '
        RELEEM_TEST_MODE=1 source "$1"
        postgresql_root_exec() {
            printf "%s\n" "$*" >&2
            case "$*" in
                *"SHOW server_version_num"*) printf "140000\n" ;;
                *"pg_database"*) printf "first\000second\000" ;;
                *"pg_namespace"*) printf "app\000" ;;
                *"ALTER ROLE"*) return 44 ;;
                *"GRANT"*) [[ "$*" == *"-v ON_ERROR_STOP=1"* ]] || return 93 ;;
            esac
        }
        grant_postgresql_query_optimization_access postgres releem
    ' _ "${INSTALL_SH}"

    [ "$status" -ne 0 ]
    [[ "$output" == *'ALTER ROLE "releem" IN DATABASE "first" SET search_path'* ]]
}
