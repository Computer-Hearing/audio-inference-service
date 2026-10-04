package main

import (
	"audio-inference-service/gen/inference/v1/inferencev1connect"
	"audio-inference-service/internal/config"
	"audio-inference-service/internal/modules/catalog"
	"audio-inference-service/internal/modules/models"
	"audio-inference-service/internal/modules/predictor"
	"audio-inference-service/internal/modules/sqlite"
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	db, err := pkg.SqliteOpen(cfg.DBPath, nil)
	if err != nil {
		logger.Error("failed to open sqlite database", "err", err.Error())
		os.Exit(1)
	}
	defer db.Close()
	logger.Info("sqlite database opened", "db_path", cfg.DBPath)

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
	logger.Info("triton connect sucess", "addr", cfg.TritonAddr)

	modelCatalog := catalog.New(tritonClient, 30*time.Second)

	// хранение моделей
	storage := models.NewStorage()
	syncer := models.NewSyncer(storage, modelCatalog, 5*time.Second, logger)
	go func() {
		syncer.Run(ctx)
	}()

	taskManager := sqlite.New(db)
	predict := predictor.New(&predictor.Options{
		TritonConnector: tritonClient,
		TaskManager:     taskManager,
	})

	// Запускаем воркеры и диспетчера задач
	taskpipe.StartPipeline(ctx, taskManager, predict)

	h := handlers.New(&handlers.Options{
		TaskLoader:   taskManager,
		Catalog:      modelCatalog,
		ModelStorage: storage,
		Logger:       logger,
	})
	path, connectHandler := inferencev1connect.NewInferenceServiceHandler(h,
		connect.WithInterceptors(handlers.NewUsernameInterceptor()),
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

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http server shutdown failed", "err", err.Error())
	}
}

func printConfig(cfg *config.Config, logger *slog.Logger) {
	if cfg == nil {
		fmt.Println("config is nil")
		return
	}

	logger.Debug("config",
		slog.String("ENV", cfg.Env),
		slog.String("LogLevel", cfg.LogLevel),
		slog.String("HTTP_ADDR", cfg.HTTPAddr),
		slog.String("DB_PATH", cfg.DBPath),
		slog.String("TRITON_ADDR", cfg.TritonAddr))
}
