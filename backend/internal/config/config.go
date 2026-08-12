package config

import (
	"fmt"

	"github.com/caarlos0/env/v10"
	"github.com/go-playground/validator/v10"
)

// Config собирает настройки сервиса из переменных окружения
type Config struct {
	Env        string `env:"ENV" envDefault:"development" validate:"omitempty,oneof=development production"`
	LogLevel   string `env:"LOG_LEVEL" envDefault:"info" validate:"omitempty,oneof=debug info warn error"`
	HTTPAddr   string `env:"HTTP_ADDR" envDefault:":6767" validate:"required"`
	DBPath     string `env:"DB_PATH" envDefault:"./sqlite.db" validate:"required"`
	TritonAddr string `env:"TRITON_ADDR" envDefault:"localhost:8001" validate:"required"`
}

// Load читает переменные окружения и валидирует конфигурацию
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse env config: %w", err)
	}

	if err := validator.New().Struct(cfg); err != nil {
		return nil, fmt.Errorf("validate env config: %w", err)
	}

	return cfg, nil
}
