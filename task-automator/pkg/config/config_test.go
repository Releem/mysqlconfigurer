package config

import (
	"os"
	"testing"
)

func TestLoadConfigDefaults(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig() with empty path should not error: %v", err)
	}

	if cfg.BackupDir != "/tmp/backups" {
		t.Errorf("LoadConfig() BackupDir = %v, want /tmp/backups", cfg.BackupDir)
	}

	if cfg.PTOSCPath != "pt-online-schema-change" {
		t.Errorf("LoadConfig() PTOSCPath = %v, want pt-online-schema-change", cfg.PTOSCPath)
	}

	if cfg.MysqldumpPath != "mysqldump" {
		t.Errorf("LoadConfig() MysqldumpPath = %v, want mysqldump", cfg.MysqldumpPath)
	}

	if cfg.XtrabackupPath != "xtrabackup" {
		t.Errorf("LoadConfig() XtrabackupPath = %v, want xtrabackup", cfg.XtrabackupPath)
	}

	if cfg.BackupSpaceBuffer != 20.0 {
		t.Errorf("LoadConfig() BackupSpaceBuffer = %v, want 20.0", cfg.BackupSpaceBuffer)
	}
}

func TestLoadConfigFromFile(t *testing.T) {
	// Create a temporary config file
	tmpFile := "/tmp/test-config.json"
	defer os.Remove(tmpFile)

	testConfig := &Config{
		BackupDir:         "/custom/backups",
		PTOSCPath:         "/usr/local/bin/pt-online-schema-change",
		MysqldumpPath:     "/usr/bin/mysqldump",
		XtrabackupPath:    "/usr/bin/xtrabackup",
		BackupSpaceBuffer: 30.0,
	}

	// Save config
	if err := SaveConfig(testConfig, tmpFile); err != nil {
		t.Fatalf("SaveConfig() failed: %v", err)
	}

	// Load config
	cfg, err := LoadConfig(tmpFile)
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	if cfg.BackupDir != "/custom/backups" {
		t.Errorf("LoadConfig() BackupDir = %v, want /custom/backups", cfg.BackupDir)
	}

	if cfg.PTOSCPath != "/usr/local/bin/pt-online-schema-change" {
		t.Errorf("LoadConfig() PTOSCPath = %v, want /usr/local/bin/pt-online-schema-change", cfg.PTOSCPath)
	}

	if cfg.BackupSpaceBuffer != 30.0 {
		t.Errorf("LoadConfig() BackupSpaceBuffer = %v, want 30.0", cfg.BackupSpaceBuffer)
	}
}

