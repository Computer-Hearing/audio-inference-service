package models

import (
	"sync"
)

// ModelConfig описание модели инференса.
type ModelConfig struct {
	Name               string
	ProjectTitle       string
	ProjectDescription string
	SecondsPerChunk    int
	TargetClassesNum   int
	CategoryClassesNum int
	Ready              bool
	State              string
}

// Storage хранилище моделей в памяти.
type Storage struct {
	mu     sync.RWMutex
	models map[string]ModelConfig
}

// NewStorage создаёт пустое хранилище.
func NewStorage() *Storage {
	return &Storage{
		models: make(map[string]ModelConfig),
	}
}

// Get возвращает конфиг модели по имени.
func (s *Storage) Get(name string) (ModelConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cfg, ok := s.models[name]
	return cfg, ok
}

// GetAll возвращает все конфиги моделей.
func (s *Storage) GetAll() []ModelConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]ModelConfig, 0, len(s.models))
	for _, cfg := range s.models {
		result = append(result, cfg)
	}
	return result
}

// Set добавляет или обновляет конфиг модели.
func (s *Storage) Set(cfg ModelConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.models[cfg.Name] = cfg
}

// ReplaceAll атомарно заменяет всё хранилище новой мапой.
func (s *Storage) ReplaceAll(fresh map[string]ModelConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.models = fresh
}

// Names возвращает список имён моделей.
func (s *Storage) Names() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.models))
	for name := range s.models {
		names = append(names, name)
	}
	return names
}
