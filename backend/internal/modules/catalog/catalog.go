package catalog

import (
	"audio-inference-service/internal/modules/triton"
	"audio-inference-service/pkg"
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

const defaultCacheTTL = 30 * time.Second

// IOInfo описание тензора входа/выхода модели
type IOInfo struct {
	Name     string  `json:"name"`
	Datatype string  `json:"datatype"`
	Shape    []int64 `json:"shape,omitempty"`
}

// ModelInfo публичное описание модели для API
type ModelInfo struct {
	Name    string   `json:"name"`
	Backend string   `json:"backend,omitempty"`
	Version string   `json:"version,omitempty"`
	State   string   `json:"state,omitempty"`
	Ready   bool     `json:"ready"`
	Usable  bool     `json:"usable"`
	Inputs  []IOInfo `json:"inputs,omitempty"`
	Outputs []IOInfo `json:"outputs,omitempty"`
}

// TritonCatalog получает каталог моделей из Тритона по gRPC и кэширует его
type TritonCatalog struct {
	client *triton.TritonClient
	ttl    time.Duration

	mu       sync.Mutex
	cached   []ModelInfo
	cachedAt time.Time
}

func NewTritonCatalog(client *triton.TritonClient, ttl time.Duration) *TritonCatalog {
	if ttl <= 0 {
		ttl = defaultCacheTTL
	}

	return &TritonCatalog{
		client: client,
		ttl:    ttl,
	}
}

func (c *TritonCatalog) List(ctx context.Context) ([]ModelInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cached != nil && time.Since(c.cachedAt) < c.ttl {
		return c.cached, nil
	}

	models, err := c.fetch(ctx)
	if err != nil {
		// Если кэш уже есть, отдаём его (устаревший), иначе ошибка
		if c.cached != nil {
			slog.Warn("failed to refresh model catalog, serving stale cache", "err", err.Error())
			return c.cached, nil
		}
		return nil, err
	}

	c.cached = models
	c.cachedAt = time.Now()
	return models, nil
}

func (c *TritonCatalog) IsAvailable(ctx context.Context, modelName string) (bool, error) {
	models, err := c.List(ctx)
	if err != nil {
		return false, err
	}

	for _, m := range models {
		if m.Name == modelName {
			return m.Usable, nil
		}
	}
	return false, nil
}

func (c *TritonCatalog) fetch(ctx context.Context) ([]ModelInfo, error) {
	index, err := c.client.RepositoryIndex(ctx)
	if err != nil {
		return nil, err
	}

	infos := make([]ModelInfo, len(index))

	g, gctx := errgroup.WithContext(ctx)
	sem := make(chan struct{}, pkg.MaxTritonConcurrency)

	for i, m := range index {
		sem <- struct{}{}
		g.Go(func() error {
			defer func() { <-sem }()
			infos[i] = c.modelInfo(gctx, m)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Оставляем только модели, которые сервис реально может вызвать
	usable := infos[:0]
	for _, m := range infos {
		if m.Usable {
			usable = append(usable, m)
		}
	}

	sort.Slice(usable, func(a, b int) bool { return usable[a].Name < usable[b].Name })

	return usable, nil
}

func (c *TritonCatalog) modelInfo(ctx context.Context, m triton.RepositoryIndexModel) ModelInfo {
	ready := m.State == "READY"
	info := ModelInfo{
		Name:    m.Name,
		Version: m.Version,
		State:   m.State,
		Ready:   ready,
	}

	// Модель может не загрузиться в Тритон — тогда конфиг не отдастся,
	// но сам факт существования покажем (usable=false)
	cfg, err := c.client.ModelConfig(ctx, m.Name)
	if err != nil {
		slog.Debug("failed to get model config", "model", m.Name, "err", err.Error())
		return info
	}

	info.Backend = cfg.Backend

	for _, in := range cfg.Inputs {
		info.Inputs = append(info.Inputs, IOInfo{Name: in.Name, Datatype: in.Datatype, Shape: in.Shape})
		if in.Name == pkg.RawAudioInputName && in.Datatype == pkg.RawAudioInputDatatype {
			info.Usable = true
		}
	}
	for _, out := range cfg.Outputs {
		info.Outputs = append(info.Outputs, IOInfo{Name: out.Name, Datatype: out.Datatype, Shape: out.Shape})
	}

	return info
}
