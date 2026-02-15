package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds the daemon configuration.
type Config struct {
	ListenAddr string `json:"listen_addr"` // default ":8042"
	DataDir    string `json:"data_dir"`    // default "~/.boxofrocks"
	DBPath     string `json:"db_path"`     // default "{data_dir}/bor.db"
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".boxofrocks")
	return &Config{
		ListenAddr: ":8042",
		DataDir:    dataDir,
		DBPath:     filepath.Join(dataDir, "bor.db"),
	}
}

// configPath returns the path to the config file.
func configPath(cfg *Config) string {
	return filepath.Join(cfg.DataDir, "config.json")
}

// expandHome replaces a leading "~" with the user's home directory.
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[1:])
	}
	return path
}

// Load reads configuration from ~/.boxofrocks/config.json.
// If the file does not exist, it returns the default configuration.
func Load() (*Config, error) {
	cfg := DefaultConfig()
	path := configPath(cfg)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Expand home directory references.
	cfg.DataDir = expandHome(cfg.DataDir)
	cfg.DBPath = expandHome(cfg.DBPath)

	// If DBPath is empty after loading, set the default relative to DataDir.
	if cfg.DBPath == "" {
		cfg.DBPath = filepath.Join(cfg.DataDir, "bor.db")
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return cfg, nil
}

// Validate checks that the Config contains valid values.
func (c *Config) Validate() error {
	if c.ListenAddr == "" {
		return fmt.Errorf("listen_addr must not be empty")
	}

	// Extract and validate the port.
	_, portStr, err := net.SplitHostPort(c.ListenAddr)
	if err != nil {
		return fmt.Errorf("invalid listen_addr %q: %w", c.ListenAddr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid port in listen_addr %q: %w", c.ListenAddr, err)
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("port %d out of range (1-65535)", port)
	}

	if c.DataDir == "" {
		return fmt.Errorf("data_dir must not be empty")
	}

	return nil
}

// Save writes the configuration to ~/.boxofrocks/config.json.
func Save(cfg *Config) error {
	if err := EnsureDataDir(cfg); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	path := configPath(cfg)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

// EnsureDataDir creates the data directory if it does not exist.
func EnsureDataDir(cfg *Config) error {
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return fmt.Errorf("create data dir %s: %w", cfg.DataDir, err)
	}
	return nil
}
