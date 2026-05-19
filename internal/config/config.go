// Пакет config загружает конфигурацию приложения из YAML-файла.
// Реализуйте этот пакет самостоятельно.
package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config содержит параметры запуска сервера.
// Изучите config.yaml и добавьте поля самостоятельно.
type Config struct {
	ServerHost             string `yaml:"server_host"`
	ServerPort             int    `yaml:"server_port"`
	LogLevel               string `yaml:"log_level"`
	AccrualIntervalSeconds int    `yaml:"accrual_interval_seconds"`
	WorkerConcurrency      int    `yaml:"worker_concurrency"`
}

// Load читает конфигурацию из файла config.yaml.
// Если файл не найден или поле не задано, применяются значения по умолчанию.
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
