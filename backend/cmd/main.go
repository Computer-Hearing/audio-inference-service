package main

import (
	"audio-inference-service/gen/inference/v1/inferencev1connect"
	"audio-inference-service/internal/config"
	"audio-inference-service/internal/modules/catalog"
	"audio-inference-service/internal/modules/models"
	"audio-inference-service/internal/modules/predictor"
	taskredis "audio-inference-service/internal/modules/redis"
	"audio-inference-service/internal/modules/taskpipe"
	"audio-inference-service/internal/modules/triton"
	handlers "audio-inference-service/internal/server/connect"
	"audio-inference-service/pkg"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"connectrpc.com/connect"
	goredis "github.com/redis/go-redis/v9"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "err", err.Error())
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout,
		&slog.HandlerOptions{Level: pkg.GetLoglevel(cfg.LogLevel), AddSource: pkg.GetLoglevel(cfg.LogLevel) == slog.LevelDebug}))
	slog.SetDefault(logger)

	printConfig(cfg, logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	redisClient := goredis.NewClient(&goredis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	defer redisClient.Close()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		logger.Error("failed to connect to redis", "addr", cfg.RedisAddr, "err", err.Error())
		os.Exit(1)
	}
	logger.Info("redis connected", "addr", cfg.RedisAddr)

	tritonClient, err := triton.NewTritonClient(triton.DefaultConfig(cfg.TritonAddr))
	if err != nil {
		logger.Error("triton client error", "error", err.Error())
		os.Exit(1)
	}
	defer tritonClient.Close()

	if err := tritonClient.Connect(ctx); err != nil {
		logger.Error("failed to connect to triton", "addr", cfg.TritonAddr, "err", err.Error())
		os.Exit(1)
	}
	logger.Info("triton connect success", "addr", cfg.TritonAddr)

	modelCatalog := catalog.New(tritonClient, 30*time.Second)

	// хранение моделей
	storage := models.NewStorage()
	syncer := models.NewSyncer(storage, modelCatalog, 5*time.Minute, logger)
	go func() {
		syncer.Run(ctx)
	}()

	taskManager := taskredis.New(&taskredis.Options{
		Client:   redisClient,
		Stream:   cfg.TaskStream,
		Group:    cfg.TaskGroup,
		Consumer: consumerName(),
		TaskTTL:  cfg.TaskTTL,
	})
	if err := taskManager.EnsureGroup(ctx); err != nil {
		logger.Error("failed to create task consumer group", "err", err.Error())
		os.Exit(1)
	}

	predict := predictor.New(&predictor.Options{
		TritonConnector: tritonClient,
		TaskManager:     taskManager,
		Logger:          logger,
	})

	// Запускаем воркеры, читателя очереди и восстановление зависших задач
	taskpipe.StartPipeline(ctx, taskpipe.Options{
		Logger:        logger,
		Store:         taskManager,
		Queue:         taskManager,
		Predictor:     predict,
		AckWait:       cfg.TaskAckWait,
		MaxDeliveries: cfg.TaskMaxDeliveries,
	})

	h := handlers.New(&handlers.Options{
		TaskLoader:   taskManager,
		Catalog:      modelCatalog,
		ModelStorage: storage,
		Logger:       logger,
	})
	path, connectHandler := inferencev1connect.NewInferenceServiceHandler(h,
		connect.WithReadMaxBytes(4<<20+64<<10),
	)

	mux := http.NewServeMux()
	mux.Handle(path, connectHandler)

	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 5 * time.Minute,
		Protocols:    &http.Protocols{},
	}
	srv.Protocols.SetHTTP1(true)
	srv.Protocols.SetUnencryptedHTTP2(true)

	go func() {
		logger.Info("http server started", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server failed", "err", err.Error())
			stop()
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down http server")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http server shutdown failed", "err", err.Error())
	}
}

// consumerName уникален для каждого запуска процесса: старые записи в PEL
// остаются за прежним consumer'ом и подбираются через XAUTOCLAIM.
func consumerName() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "backend"
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}

func printConfig(cfg *config.Config, logger *slog.Logger) {
	if cfg == nil {
		logger.Debug("config is nil")
		return
	}

	logger.Debug("config",
		slog.String("ENV", cfg.Env),
		slog.String("LogLevel", cfg.LogLevel),
		slog.String("HTTP_ADDR", cfg.HTTPAddr),
		slog.String("REDIS_ADDR", cfg.RedisAddr),
		slog.Duration("TASK_TTL", cfg.TaskTTL),
		slog.String("TRITON_ADDR", cfg.TritonAddr))
}
