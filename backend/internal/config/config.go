package config

import (
	"fmt"
	"time"

	"github.com/joho/godotenv"

	"github.com/caarlos0/env/v10"
	"github.com/go-playground/validator/v10"
)

// Config собирает настройки сервиса из переменных окружения
type Config struct {
	Env        string `env:"ENV" envDefault:"development" validate:"omitempty,oneof=development production"`
	LogLevel   string `env:"LOG_LEVEL" envDefault:"debug" validate:"omitempty,oneof=debug info warn error"`
	HTTPAddr   string `env:"SERVER_ADDR" envDefault:":6767" validate:"required"`
	TritonAddr string `env:"TRITON_ADDR" envDefault:"triton:8001" validate:"required"`

	// Redis: очередь и хранилище состояния задач.
	RedisAddr     string `env:"REDIS_ADDR" envDefault:"redis:6379" validate:"required"`
	RedisPassword string `env:"REDIS_PASSWORD" envDefault:""`
	RedisDB       int    `env:"REDIS_DB" envDefault:"0"`

	// TaskTTL — сколько живёт задача (и её результат) после последнего обновления.
	TaskTTL time.Duration `env:"TASK_TTL" envDefault:"1h" validate:"required"`

	// Параметры очереди на Redis Streams.
	TaskStream        string        `env:"TASK_STREAM" envDefault:"tasks" validate:"required"`
	TaskGroup         string        `env:"TASK_GROUP" envDefault:"inference" validate:"required"`
	TaskAckWait       time.Duration `env:"TASK_ACK_WAIT" envDefault:"10m" validate:"required"`
	TaskMaxDeliveries int           `env:"TASK_MAX_DELIVERIES" envDefault:"3" validate:"required,gt=0"`
}

// Load читает переменные окружения и валидирует конфигурацию
func Load() (*Config, error) {
	cfg := &Config{}

	_ = godotenv.Load(".env")

	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse env config: %w", err)
	}

	if err := validator.New().Struct(cfg); err != nil {
		return nil, fmt.Errorf("validate env config: %w", err)
	}

	return cfg, nil
}
