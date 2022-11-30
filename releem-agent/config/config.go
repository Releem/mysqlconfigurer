package config

import (
	"io/ioutil"
	"time"

	"github.com/advantageous/go-logback/logging"
	"github.com/hashicorp/hcl"
)

type Config struct {
	Debug                 bool          `hcl:"debug"`
	Env                   string        `hcl:"env"`
	ApiKey                string        `hcl:"apikey"`
	TimePeriodSeconds     time.Duration `hcl:"interval_seconds"`
	ReadConfigSeconds     time.Duration `hcl:"interval_read_config_seconds"`
	GenerateConfigSeconds time.Duration `hcl:"interval_generate_config_seconds"`
	MysqlPassword         string        `hcl:"mysql_password"`
	MysqlUser             string        `hcl:"mysql_user"`
	MysqlHost             string        `hcl:"mysql_host"`
	MysqlPort             string        `hcl:"mysql_port"`
	CommandRestartService string        `hcl:"mysql_restart_service"`
	MysqlConfDir          string        `hcl:"mysql_cnf_dir"`
	ReleemConfDir         string        `hcl:"releem_cnf_dir"`
}

func LoadConfig(filename string, logger logging.Logger) (*Config, error) {
	if logger == nil {
		logger = logging.NewSimpleLogger("config")
	}
	logger.Printf("Loading config %s", filename)
	configBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return LoadConfigFromString(string(configBytes), logger)
}

func LoadConfigFromString(data string, logger logging.Logger) (*Config, error) {
	if logger == nil {
		logger = logging.NewSimpleLogger("config")
	}
	config := &Config{}
	err := hcl.Decode(&config, data)
	if err != nil {
		return nil, err
	}
	if config.TimePeriodSeconds == 0 {
		config.TimePeriodSeconds = 60
	}
	if config.ReadConfigSeconds == 0 {
		config.ReadConfigSeconds = 3600
	}
	if config.GenerateConfigSeconds == 0 {
		config.GenerateConfigSeconds = 43200
	}
	if config.MysqlHost == "" {
		config.MysqlHost = "127.0.0.1"
	}
	if config.MysqlPort == "" {
		config.MysqlPort = "3306"
	}
	return config, nil
}

func (config *Config) GetApiKey() string {
	return config.ApiKey
}
func (config *Config) GetEnv() string {
	return config.Env
}