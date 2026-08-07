package triton

import (
	"context"
	"fmt"
	"sync"
	"time"

	pb "audio-inference-service/gen"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Config конфигурация подключения
type Config struct {
	ServerAddress  string
	Timeout        time.Duration
	MaxRetries     int
	RetryDelay     time.Duration
	MaxRecvMsgSize int
	MaxSendMsgSize int
}

// DefaultConfig возвращает конфиг по умолчанию
func DefaultConfig(addr string) *Config {
	return &Config{
		ServerAddress:  addr,
		Timeout:        30 * time.Second,
		MaxRetries:     3,
		RetryDelay:     1 * time.Second,
		MaxRecvMsgSize: 1024 * 1024 * 1024, // 1GB
		MaxSendMsgSize: 1024 * 1024 * 20,   // 20MB
	}
}

// TritonClient структура клиента для подключения к Triton
type TritonClient struct {
	mu          sync.RWMutex
	client      pb.GRPCInferenceServiceClient
	conn        *grpc.ClientConn
	config      *Config
	isConnected bool

	// метрики
	requestsTotal   int64
	requestsFailed  int64
	lastRequestTime time.Time
}

// NewTritonClient создает новый экземпляр клиента
func NewTritonClient(cfg *Config) (*TritonClient, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	return &TritonClient{
		config:      cfg,
		isConnected: false,
	}, nil
}

// Connect устанавливает соединение с Triton сервером
func (c *TritonClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isConnected {
		return nil
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(c.config.MaxRecvMsgSize),
			grpc.MaxCallSendMsgSize(c.config.MaxSendMsgSize),
		),
	}

	dialCtx, cancel := context.WithTimeout(ctx, c.config.Timeout)
	defer cancel()

	// grpc.DialContext с блокировкой до установления соединения
	conn, err := grpc.DialContext(dialCtx, c.config.ServerAddress,
		append(opts, grpc.WithBlock())...)
	if err != nil {
		return fmt.Errorf("failed to connect to Triton: %w", err)
	}

	c.conn = conn
	c.client = pb.NewGRPCInferenceServiceClient(conn)
	c.isConnected = true

	return nil
}

// Close закрывает соединение
func (c *TritonClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.isConnected {
		return nil
	}

	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			return err
		}
	}

	c.isConnected = false
	c.client = nil
	return nil
}

// IsConnected возвращает статус соединения
func (c *TritonClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isConnected
}

// Ping проверяет доступность сервера (ServerLive)
func (c *TritonClient) Ping(ctx context.Context) error {
	c.mu.RLock()
	client := c.client
	connected := c.isConnected
	c.mu.RUnlock()

	if !connected {
		return fmt.Errorf("client not connected")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := client.ServerLive(ctx, &pb.ServerLiveRequest{})
	if err != nil {
		return fmt.Errorf("server live check failed: %w", err)
	}

	if !resp.Live {
		return fmt.Errorf("server is not live")
	}

	return nil
}

// Ready проверяет готовность сервера (ServerReady) —
// в Triton это отдельный от Live эндпоинт: Live значит процесс запущен,
// Ready значит все модели загружены и сервер готов обрабатывать запросы
func (c *TritonClient) Ready(ctx context.Context) error {
	c.mu.RLock()
	client := c.client
	connected := c.isConnected
	c.mu.RUnlock()

	if !connected {
		return fmt.Errorf("client not connected")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := client.ServerReady(ctx, &pb.ServerReadyRequest{})
	if err != nil {
		return fmt.Errorf("server ready check failed: %w", err)
	}

	if !resp.Ready {
		return fmt.Errorf("server is not ready")
	}

	return nil
}

// ModelReady проверяет готовность конкретной модели
func (c *TritonClient) ModelReady(ctx context.Context, modelName, modelVersion string) (bool, error) {
	c.mu.RLock()
	client := c.client
	connected := c.isConnected
	c.mu.RUnlock()

	if !connected {
		return false, fmt.Errorf("client not connected")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := client.ModelReady(ctx, &pb.ModelReadyRequest{
		Name:    modelName,
		Version: modelVersion,
	})
	if err != nil {
		return false, fmt.Errorf("model ready check failed: %w", err)
	}

	return resp.Ready, nil
}

// Infer выполняет инференс с retry-логикой
func (c *TritonClient) Infer(ctx context.Context, req *pb.ModelInferRequest) (*pb.ModelInferResponse, error) {
	c.mu.RLock()
	client := c.client
	connected := c.isConnected
	c.mu.RUnlock()

	if !connected {
		return nil, fmt.Errorf("client not connected")
	}

	var lastErr error
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(c.config.RetryDelay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		callCtx, cancel := context.WithTimeout(ctx, c.config.Timeout)
		resp, err := client.ModelInfer(callCtx, req)
		cancel()

		c.mu.Lock()
		c.requestsTotal++
		c.lastRequestTime = time.Now()
		if err != nil {
			c.requestsFailed++
		}
		c.mu.Unlock()

		if err == nil {
			return resp, nil
		}

		lastErr = err
	}

	return nil, fmt.Errorf("inference failed after %d attempts: %w", c.config.MaxRetries+1, lastErr)
}

// Stats возвращает статистику запросов
func (c *TritonClient) Stats() (total, failed int64, lastRequest time.Time) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.requestsTotal, c.requestsFailed, c.lastRequestTime
}

// RepositoryIndexModel запись индекса репозитория моделей
type RepositoryIndexModel struct {
	Name    string
	Version string
	State   string
	Reason  string
}

// TritonTensor описание входного/выходного тензора модели
type TritonTensor struct {
	Name     string
	Datatype string // "TYPE_UINT8" и т.п.
	Shape    []int64
}

// TritonModelConfig конфигурация модели
type TritonModelConfig struct {
	Name         string
	Backend      string
	MaxBatchSize int32
	Inputs       []TritonTensor
	Outputs      []TritonTensor
}

// RepositoryIndex возвращает список моделей из репозитория Тритона
func (c *TritonClient) RepositoryIndex(ctx context.Context) ([]RepositoryIndexModel, error) {
	client, err := c.currentClient()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, c.config.Timeout)
	defer cancel()

	resp, err := client.RepositoryIndex(ctx, &pb.RepositoryIndexRequest{})
	if err != nil {
		return nil, fmt.Errorf("repository index failed: %w", err)
	}

	models := make([]RepositoryIndexModel, 0, len(resp.Models))
	for _, m := range resp.Models {
		models = append(models, RepositoryIndexModel{
			Name:    m.GetName(),
			Version: m.GetVersion(),
			State:   m.GetState(),
			Reason:  m.GetReason(),
		})
	}

	return models, nil
}

// ModelConfig возвращает конфигурацию модели по её имени
func (c *TritonClient) ModelConfig(ctx context.Context, modelName string) (*TritonModelConfig, error) {
	client, err := c.currentClient()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, c.config.Timeout)
	defer cancel()

	resp, err := client.ModelConfig(ctx, &pb.ModelConfigRequest{Name: modelName})
	if err != nil {
		return nil, fmt.Errorf("model config for %s failed: %w", modelName, err)
	}

	cfg := resp.GetConfig()
	if cfg == nil {
		return nil, fmt.Errorf("model config for %s is empty", modelName)
	}

	result := &TritonModelConfig{
		Name:         cfg.GetName(),
		Backend:      cfg.GetBackend(),
		MaxBatchSize: cfg.GetMaxBatchSize(),
	}

	for _, in := range cfg.GetInput() {
		result.Inputs = append(result.Inputs, TritonTensor{
			Name:     in.GetName(),
			Datatype: in.GetDataType().String(),
			Shape:    in.GetDims(),
		})
	}
	for _, out := range cfg.GetOutput() {
		result.Outputs = append(result.Outputs, TritonTensor{
			Name:     out.GetName(),
			Datatype: out.GetDataType().String(),
			Shape:    out.GetDims(),
		})
	}

	return result, nil
}

// currentClient возвращает активный gRPC-клиент
func (c *TritonClient) currentClient() (pb.GRPCInferenceServiceClient, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.isConnected {
		return nil, fmt.Errorf("client not connected")
	}
	return c.client, nil
}
