#!/bin/bash
set -e
# # Substitute environment variables in Prosody configs
envsubst < /docker/releem.conf.tpl > /opt/releem/releem.conf

echo -e "### This configuration was recommended by Releem. https://releem.com\n[mysqld]\nperformance_schema = 1\nslow_query_log = 1" > "/etc/mysql/releem.conf.d/collect_metrics.cnf"
if [ -n "$RELEEM_QUERY_OPTIMIZATION" -a "$RELEEM_QUERY_OPTIMIZATION" = true ]; then       
    echo "performance-schema-consumer-events-statements-history = ON" | tee -a "/etc/mysql/releem.conf.d/collect_metrics.cnf" >/dev/null
    echo "performance-schema-consumer-events-statements-current = ON" | tee -a "/etc/mysql/releem.conf.d/collect_metrics.cnf" >/dev/null
    echo "performance_schema_events_statements_history_size = 500" | tee -a "/etc/mysql/releem.conf.d/collect_metrics.cnf" >/dev/null
fi

/opt/releem/releem-agent -f

exec "$@"
