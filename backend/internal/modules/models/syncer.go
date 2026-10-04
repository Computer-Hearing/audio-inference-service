package models

import (
	"audio-inference-service/internal/modules/catalog"
	"audio-inference-service/pkg"
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Syncer периодически обновляет Storage из Triton.
type Syncer struct {
	storage  *Storage
	catalog  *catalog.TritonCatalog
	interval time.Duration
	logger   *slog.Logger
}

// NewSyncer создаёт новый Syncer.
func NewSyncer(storage *Storage, catalog *catalog.TritonCatalog, interval time.Duration, logger *slog.Logger) *Syncer {
	return &Syncer{
		storage:  storage,
		catalog:  catalog,
		interval: interval,
		logger:   logger,
	}
}

// Run запускает цикл синхронизации. Блокируется до завершения ctx.
func (s *Syncer) Run(ctx context.Context) {
	// Первый refresh блокирующий — иначе /models отдаст пустоту
	if err := s.refresh(ctx); err != nil {
		s.logger.Warn("initial modelsync failed", "error", err.Error())
	}

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.refresh(ctx); err != nil {
				// Не чистим storage, оставляем прошлое
				s.logger.Warn("modelsync refresh failed", "error", err.Error())
			}
		}
	}
}

// refresh обновляет Storage из Triton.
func (s *Syncer) refresh(ctx context.Context) error {
	infos, err := s.catalog.List(ctx)
	if err != nil {
		return fmt.Errorf("catalog list: %w", err)
	}

	fresh := make(map[string]ModelConfig, len(infos))
	for _, m := range infos {
		name, spc, ok := ParseModelName(m.Name)
		if !ok {
			s.logger.Debug("skipping model with invalid name", "name", m.Name)
			continue
		}

		cfg := ModelConfig{
			Name:               name,
			ProjectTitle:       fmt.Sprintf("Project for %s", name),
			SecondsPerChunk:    spc,
			TargetClassesNum:   outputLen(m, pkg.TargetOutputName),
			CategoryClassesNum: outputLen(m, pkg.CategoryOutputName),
			Ready:              m.Ready,
			State:              m.State,
		}

		// Если модель уже есть — сохраняем ручные правки title/description
		if old, ok := s.storage.Get(name); ok {
			cfg.ProjectTitle = old.ProjectTitle
			cfg.ProjectDescription = old.ProjectDescription
		}

		fresh[name] = cfg
	}
	s.storage.ReplaceAll(fresh)
	s.logger.Info("models synced", "count", len(fresh))
	return nil
}

// outputLen возвращает длину выходного тензора по имени.
func outputLen(m catalog.ModelInfo, outputName string) int {
	for _, out := range m.Outputs {
		if out.Name == outputName {
			// Shape[0] — размер батча, Shape[1] — количество классов
			if len(out.Shape) >= 2 {
				return int(out.Shape[1])
			}
			return 0
		}
	}
	return 0
}
