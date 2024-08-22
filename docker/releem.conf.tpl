# ApiKey string `hcl:"apikey"`
# Defaults to 3600 seconds, api key for Releem Platform.
apikey="${RELEEM_API_KEY}"

hostname="${RELEEM_HOSTNAME}"

# MemoryLimit int `hcl:"memory_limit"`
# Defaults to 0, Mysql memory usage limit.
memory_limit=${MEMORY_LIMIT:-0}

# MetricsPeriod time.Duration `hcl:"interval_seconds"`
# Defaults to 30 seconds, how often metrics are collected.
interval_seconds=60

# ReadConfigPeriod time.Duration `hcl:"interval_read_config_seconds"`
# Defaults to 3600 seconds, how often to update the values from the config.
interval_read_config_seconds=3600

# GenerateConfigPeriod time.Duration `hcl:"interval_generate_config_seconds"`
# Defaults to 43200 seconds, how often to generate recommend the config.
interval_generate_config_seconds=${RELEEM_INTERVAL_COLLECT_ALL_METRICS:-43200}

# QueryOptimization time.Duration `hcl:"interval_query_optimization_seconds"`
# Defaults to 3600 seconds, how often query metrics are collected.
interval_query_optimization_seconds=3600

# MysqlUser string`hcl:"mysql_user"`
# Mysql user name for collection metrics.
mysql_user="${DB_USER:-releem}"

# MysqlPassword string `hcl:"mysql_password"`
# Mysql user password for collection metrics.
mysql_password="${DB_PASSWORD:-releem}"

# MysqlHost string `hcl:"mysql_host"`
# Mysql host for collection metrics.
mysql_host="${DB_HOST:-127.0.0.1}"

# MysqlPort string `hcl:"mysql_port"`
# Mysql port for collection metrics.
mysql_port="${DB_PORT:-3306}"

# CommandRestartService string `hcl:"mysql_restart_service"`
# Defaults to 3600 seconds, command to restart service mysql.
mysql_restart_service=" /bin/systemctl restart mysql"

# MysqlConfDir string `hcl:"mysql_cnf_dir"`
# The path to copy the recommended config.
mysql_cnf_dir="/etc/mysql/releem.conf.d"

# ReleemConfDir string `hcl:"releem_cnf_dir"`
# Releem Agent configuration path.
releem_cnf_dir="/opt/releem/conf"

# InstanceType string `hcl:"instance_type"`
# Defaults to local, type of instance "local" or "aws/rds"
instance_type="${INSTANCE_TYPE:-local}"

# AwsRegion string `hcl:"aws_region"`
# Defaults to us-east-1, AWS region for RDS
aws_region="${AWS_REGION:-us-east-1}"

# AwsRDSDB string `hcl:"aws_rds_db"`
# RDS database name.
aws_rds_db="${AWS_RDS_DB}"

# Env string `hcl:"env"`
# Releem Environment.
env="${RELEEM_ENV:-prod}"

# Debug string `hcl:"debug"`
# Releem Debug messages
debug=${RELEEM_DEBUG:-false}

# Collect Explain string `hcl:"query_optimization"`
# Releem collect explain for query
query_optimization=${RELEEM_QUERY_OPTIMIZATION:-false}