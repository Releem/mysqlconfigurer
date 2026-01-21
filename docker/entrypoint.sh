#!/bin/bash
set -e
# # Substitute environment variables in Prosody configs
#envsubst < /docker/releem.conf.tpl > /opt/releem/releem.conf

cat <<EOF > /opt/releem/releem.conf
apikey="${RELEEM_API_KEY}"
hostname="${RELEEM_HOSTNAME}"
memory_limit=${MEMORY_LIMIT:-0}
interval_seconds=60
interval_read_config_seconds=3600
interval_generate_config_seconds=${RELEEM_INTERVAL_COLLECT_ALL_METRICS:-43200}
interval_query_optimization_seconds=3600
mysql_user="${DB_USER:-releem}"
mysql_password="${DB_PASSWORD:-releem}"
mysql_host="${DB_HOST:-127.0.0.1}"
mysql_port="${DB_PORT:-3306}"
mysql_ssl_mode=${DB_SSL:-false}
mysql_restart_service=" /bin/systemctl restart mysql"
mysql_cnf_dir="/etc/mysql/releem.conf.d"
releem_cnf_dir="/opt/releem/conf"
instance_type="${INSTANCE_TYPE:-local}"
aws_region="${AWS_REGION}"
aws_rds_db="${AWS_RDS_DB}"
aws_rds_parameter_group="${AWS_RDS_PARAMETER_GROUP}"
gcp_project_id="${RELEEM_GCP_PROJECT_ID}"
gcp_region="${RELEEM_GCP_REGION}"
gcp_cloudsql_instance="${RELEEM_GCP_CLOUDSQL_INSTANCE}"
gcp_cloudsql_public_connection=${RELEEM_GCP_CLOUDSQL_PUBLIC_CONNECTION:-false}
env="${RELEEM_ENV:-prod}"
debug=${RELEEM_DEBUG:-false}
query_optimization=${RELEEM_QUERY_OPTIMIZATION:-false}
databases_query_optimization="${RELEEM_DATABASES_QUERY_OPTIMIZATION}"
releem_region="${RELEEM_REGION}"
EOF


echo -e "### This configuration was recommended by Releem. https://releem.com\n[mysqld]\nperformance_schema = 1\nslow_query_log = 1" > "/etc/mysql/releem.conf.d/collect_metrics.cnf"
if [ -n "$RELEEM_QUERY_OPTIMIZATION" -a "$RELEEM_QUERY_OPTIMIZATION" = true ]; then       
    echo "performance-schema-consumer-events-statements-history = ON" | tee -a "/etc/mysql/releem.conf.d/collect_metrics.cnf" >/dev/null
    echo "performance-schema-consumer-events-statements-current = ON" | tee -a "/etc/mysql/releem.conf.d/collect_metrics.cnf" >/dev/null
fi

/opt/releem/releem-agent -f

exec "$@"
