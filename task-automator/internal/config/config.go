package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
)

// Config represents the application configuration
type Config struct {
	BackupDir           string  `json:"backup_dir"`
	PTOSCPath           string  `json:"ptosc_path"`
	MysqldumpPath       string  `json:"mysqldump_path"`
	XtrabackupPath      string  `json:"xtrabackup_path"`
	BackupSpaceBuffer   float64 `json:"backup_space_buffer"` // Percentage buffer (e.g., 20.0 for 20%)
}

// LoadConfig loads configuration from a file
// If the file doesn't exist, returns default values
func LoadConfig(configPath string) (*Config, error) {
	config := &Config{
		BackupDir:         "/tmp/backups",
		PTOSCPath:         "pt-online-schema-change",
		MysqldumpPath:     "mysqldump",
		XtrabackupPath:    "xtrabackup",
		BackupSpaceBuffer: 20.0, // Default 20% buffer
	}

	// If no config path provided, return defaults
	if configPath == "" {
		return config, nil
	}

	// Try to load from file
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, return defaults
			return config, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse JSON
	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Validate and set defaults for empty values
	if config.BackupDir == "" {
		config.BackupDir = "/tmp/backups"
	}
	if config.PTOSCPath == "" {
		config.PTOSCPath = "pt-online-schema-change"
	}
	if config.MysqldumpPath == "" {
		config.MysqldumpPath = "mysqldump"
	}
	if config.XtrabackupPath == "" {
		config.XtrabackupPath = "xtrabackup"
	}
	if config.BackupSpaceBuffer == 0 {
		config.BackupSpaceBuffer = 20.0 // Default 20% buffer
	}

	return config, nil
}

// SaveConfig saves configuration to a file
func SaveConfig(config *Config, configPath string) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return ioutil.WriteFile(configPath, data, 0644)
}

