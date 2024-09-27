package config

import (
	"database/sql"
	"os"
	"sync"
	"time"

	"github.com/advantageous/go-logback/logging"
	"github.com/hashicorp/hcl"
)

const (
	ReleemAgentVersion = "1.19.3.1"
)

var (
	DB           *sql.DB
	SqlText      map[string]map[string]string
	SqlTextMutex sync.RWMutex
)

type Config struct {
	Debug                                 bool          `hcl:"debug"`
	Env                                   string        `hcl:"env"`
	Hostname                              string        `hcl:"hostname"`
	ApiKey                                string        `hcl:"apikey"`
	MetricsPeriod                         time.Duration `hcl:"interval_seconds"`
	ReadConfigPeriod                      time.Duration `hcl:"interval_read_config_seconds"`
	GenerateConfigPeriod                  time.Duration `hcl:"interval_generate_config_seconds"`
	QueryOptimizationPeriod               time.Duration `hcl:"interval_query_optimization_seconds"`
	QueryOptimizationCollectSqlTextPeriod time.Duration `hcl:"interval_query_optimization_collect_sqltext_seconds"`
	MysqlPassword                         string        `hcl:"mysql_password"`
	MysqlUser                             string        `hcl:"mysql_user"`
	MysqlHost                             string        `hcl:"mysql_host"`
	MysqlPort                             string        `hcl:"mysql_port"`
	MysqlSslMode                          bool          `hcl:"mysql_ssl_mode"`
	CommandRestartService                 string        `hcl:"mysql_restart_service"`
	MysqlConfDir                          string        `hcl:"mysql_cnf_dir"`
	ReleemConfDir                         string        `hcl:"releem_cnf_dir"`
	ReleemDir                             string        `hcl:"releem_dir"`
	MemoryLimit                           int           `hcl:"memory_limit"`
	InstanceType                          string        `hcl:"instance_type"`
	AwsRegion                             string        `hcl:"aws_region"`
	AwsRDSDB                              string        `hcl:"aws_rds_db"`
	QueryOptimization                     bool          `hcl:"query_optimization"`
}

func LoadConfig(filename string, logger logging.Logger) (*Config, error) {
	if logger == nil {
		logger = logging.NewSimpleLogger("config")
	}
	logger.Printf("Loading config %s", filename)
	configBytes, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return LoadConfigFromString(string(configBytes), logger)
}

func LoadConfigFromString(data string, logger logging.Logger) (*Config, error) {
	config := &Config{}
	err := hcl.Decode(&config, data)
	if err != nil {
		return nil, err
	}
	if config.MetricsPeriod == 0 {
		config.MetricsPeriod = 60
	}
	if config.ReadConfigPeriod == 0 {
		config.ReadConfigPeriod = 3600
	}
	if config.GenerateConfigPeriod == 0 {
		config.GenerateConfigPeriod = 43200
	}
	if config.QueryOptimizationPeriod == 0 {
		config.QueryOptimizationPeriod = 3600
	}
	if config.QueryOptimizationCollectSqlTextPeriod == 0 {
		config.QueryOptimizationCollectSqlTextPeriod = 1
	}
	if config.MysqlHost == "" {
		config.MysqlHost = "127.0.0.1"
	}
	if config.MysqlPort == "" {
		config.MysqlPort = "3306"
	}
	if config.ReleemDir == "" {
		config.ReleemDir = "/opt/releem"
	}
	return config, nil
}

func (config *Config) GetApiKey() string {
	return config.ApiKey
}
func (config *Config) GetEnv() string {
	return config.Env
}

func (config *Config) GetMemoryLimit() int {
	return config.MemoryLimit
}
func (config *Config) GetReleemConfDir() string {
	return config.ReleemConfDir
}
