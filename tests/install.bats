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
    cat >"$out" <<'"'"'INNER'"'"'
#!/usr/bin/env bash
exit 0
INNER
    chmod +x "$out"
  elif [[ "$out" == *"mysqlconfigurer.sh"* ]]; then
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

@test "detect_database_type defaults to mysql" {
    load_install_functions
    unset RELEEM_PG_HOST RELEEM_PG_LOGIN RELEEM_PG_PASSWORD RELEEM_PG_ROOT_PASSWORD
    unset RELEEM_MYSQL_HOST RELEEM_MYSQL_LOGIN RELEEM_MYSQL_PASSWORD RELEEM_MYSQL_ROOT_PASSWORD

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

@test "setup_mysql_connection_string builds host and default port" {
    load_install_functions
    unset RELEEM_MYSQL_PORT
    RELEEM_MYSQL_HOST="10.1.2.3"

    run setup_mysql_connection_string
    [ "$status" -eq 0 ]
    [[ "$connection_string" == *"--host=10.1.2.3"* ]]
    [[ "$connection_string" == *"--port=3306"* ]]
    [ "$mysql_user_host" = "%" ]
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

@test "detect_mysql_commands exits non-zero when mysql binaries are missing" {
    create_mock_cmd "which" 'exit 1'
    run env RELEEM_TEST_MODE=1 PATH="${MOCK_BIN}:/bin" bash -c "source '${INSTALL_SH}'; detect_mysql_commands"
    [ "$status" -ne 0 ]
    [[ "$output" == *"Couldn't find mysqladmin/mariadb-admin"* ]]
}

@test "aws/rds mode writes aws keys and releem_dir to releem.conf" {
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
    run grep -E "^(releem_dir|instance_type|aws_region|aws_rds_db|aws_rds_parameter_group)=" "${conf}"
    [ "$status" -eq 0 ]
    [[ "$output" == *"releem_dir=\"${workdir}\""* ]]
    [[ "$output" == *'instance_type="aws/rds"'* ]]
    [[ "$output" == *'aws_region="eu-west-1"'* ]]
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

@test "azure/mysql mode writes azure keys" {
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
    run grep -E "^(instance_type|azure_subscription_id|azure_resource_group|azure_mysql_server)=" "${conf}"
    [ "$status" -eq 0 ]
    [[ "$output" == *'instance_type="azure/mysql"'* ]]
    [[ "$output" == *'azure_subscription_id="sub-1"'* ]]
    [[ "$output" == *'azure_resource_group="rg-1"'* ]]
    [[ "$output" == *'azure_mysql_server="mysql-1"'* ]]
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

@test "enable_query_optimization mode updates releem.conf and executes mysqlconfigurer" {
    prepare_common_install_mocks
    create_mock_cmd "mysqladmin" 'echo "mysqld is alive"'
    create_mock_cmd "mariadb-admin" 'echo "mysqld is alive"'
    create_mock_cmd "mysql" '
if [[ "$*" == *"-NBe"* ]]; then
  echo "GRANT SELECT on *.* to \`releem\`@\`%\`;"
fi
exit 0
'
    create_mock_cmd "mariadb" '
if [[ "$*" == *"-NBe"* ]]; then
  echo "GRANT SELECT on *.* to \`releem\`@\`%\`;"
fi
exit 0
'

    local workdir="${TEST_TMPDIR}/workdir"
    local conf="${workdir}/releem.conf"
    mkdir -p "${workdir}"
    printf 'apikey="k1"\n' > "${conf}"
    printf '#!/usr/bin/env bash\nexit 0\n' > "${workdir}/releem-agent"
    printf '#!/usr/bin/env bash\necho "$@" >> "%s/calls.log"\nexit 0\n' "${workdir}" > "${workdir}/mysqlconfigurer.sh"
    chmod +x "${workdir}/releem-agent" "${workdir}/mysqlconfigurer.sh"
    cp "${INSTALL_SH}" "${MOCK_BIN}/enable_query_optimization"
    chmod +x "${MOCK_BIN}/enable_query_optimization"

    PATH="${MOCK_BIN}:${PATH}" run env \
        RELEEM_TEST_MODE=1 \
        RELEEM_WORKDIR="${workdir}" \
        RELEEM_CONF_FILE="${conf}" \
        RELEEM_API_KEY="k1" \
        RELEEM_MYSQL_ROOT_PASSWORD="rootpwd" \
        RELEEM_CRON_ENABLE="1" \
        enable_query_optimization

    [ "$status" -eq 0 ]
    run grep -E "^query_optimization=true$" "${conf}"
    [ "$status" -eq 0 ]
    run grep -E "^-p$" "${workdir}/calls.log"
    [ "$status" -eq 0 ]
}

@test "uninstall mode calls agent uninstall flow and rm with mocked side-effects" {
    prepare_common_install_mocks
    create_mock_cmd "rm" '
echo "$@" >> "${UNINSTALL_RM_LOG}"
exit 0
'

    local workdir="${TEST_TMPDIR}/workdir"
    local agent_calls="${TEST_TMPDIR}/agent.calls"
    local rm_calls="${TEST_TMPDIR}/rm.calls"
    mkdir -p "${workdir}"
    cat > "${workdir}/releem-agent" <<'EOF'
#!/usr/bin/env bash
echo "$@" >> "${UNINSTALL_AGENT_LOG}"
exit 0
EOF
    chmod +x "${workdir}/releem-agent"

    PATH="${MOCK_BIN}:${PATH}" run env \
        RELEEM_TEST_MODE=1 \
        RELEEM_WORKDIR="${workdir}" \
        RELEEM_API_KEY="k1" \
        UNINSTALL_AGENT_LOG="${agent_calls}" \
        UNINSTALL_RM_LOG="${rm_calls}" \
        bash "${INSTALL_SH}" uninstall

    [ "$status" -eq 0 ]
    run grep -E -- "--event=agent_uninstall|stop|remove" "${agent_calls}"
    [ "$status" -eq 0 ]
    run grep -F -- "-rf ${workdir}" "${rm_calls}"
    [ "$status" -eq 0 ]
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
  cat >"$out" <<'"'"'INNER'"'"'
#!/usr/bin/env bash
echo "$@" >> "${UPDATE_AGENT_LOG}"
exit 0
INNER
  chmod +x "$out"
fi
echo "200"
'

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
        UPDATE_CURL_LOG="${curl_calls}" \
        UPDATE_AGENT_LOG="${agent_calls}" \
        bash "${INSTALL_SH}" update

    [ "$status" -eq 0 ]
    run grep -E "releem-agent.new|mysqlconfigurer.sh.new" "${curl_calls}"
    [ "$status" -eq 0 ]
    run grep -E "stop|start|-f" "${agent_calls}"
    [ "$status" -eq 0 ]
}
