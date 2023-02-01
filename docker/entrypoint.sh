#!/bin/bash
set -e
# # Substitute environment variables in Prosody configs
envsubst < /docker/releem.conf.tpl > /opt/releem/releem.conf

echo -e "### This configuration was recommended by Releem. https://releem.com\n[mysqld]\nperformance_schema = 1\nslow_query_log = 1" > "/etc/mysql/releem.conf.d/collect_metrics.cnf"

/opt/releem/releem-agent -f

exec "$@"
