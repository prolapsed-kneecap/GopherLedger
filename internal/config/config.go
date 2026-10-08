// Package config reads the settings from a file.
package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config has the server settings.
type Config struct {
	ServerHost             string `yaml:"server_host"`
	ServerPort             int    `yaml:"server_port"`
	LogLevel               string `yaml:"log_level"`
	AccrualIntervalSeconds int    `yaml:"accrual_interval_seconds"`
	WorkerConcurrency      int    `yaml:"worker_concurrency"`
}

// Load reads config.yaml.
// It uses default values if something is missing.
func Load() (*Config, error) {
	cfg := &Config{
		ServerHost:             "localhost",
		ServerPort:             8080,
		LogLevel:               "info",
		AccrualIntervalSeconds: 3,
		WorkerConcurrency:      5,
	}

	data, err := os.ReadFile("config.yaml")
	if err != nil {
		return cfg, nil
	}

	err = yaml.Unmarshal(data, cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}
