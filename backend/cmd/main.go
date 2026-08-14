package main

import (
	"audio-inference-service/internal/config"
	"audio-inference-service/internal/modules/catalog"
	"audio-inference-service/internal/modules/predictor"
	"audio-inference-service/internal/modules/sqlite"
	"audio-inference-service/internal/modules/taskpipe"
	"audio-inference-service/internal/modules/triton"
	"audio-inference-service/internal/server/handlers"
	"audio-inference-service/internal/server/router"
	"audio-inference-service/pkg"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "err", err.Error())
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: pkg.GetLoglevel(cfg.LogLevel), AddSource: true}))
	slog.SetDefault(logger)

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
		logger.Error("failed to create triton client", "err", err.Error())
		os.Exit(1)
	}
	defer tritonClient.Close()

	if err := tritonClient.Connect(ctx); err != nil {
		logger.Error("failed to connect to triton", "addr", cfg.TritonAddr, "err", err.Error())
		os.Exit(1)
	}
	logger.Info("connected to triton", "addr", cfg.TritonAddr)

	taskManager := sqlite.NewSQLiteTaskManager(db)
	predict := &predictor.Predictor{
		TritonConnector: tritonClient,
		TaskManager:     taskManager,
	}

	// Запускаем воркеры и диспетчера задач
	taskpipe.StartPipeline(ctx, taskManager, predict)

	modelCatalog := catalog.NewTritonCatalog(tritonClient, 30*time.Second)
	handlers := handlers.New(taskManager, logger, modelCatalog)
	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      router.New(logger, handlers),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 5 * time.Minute,
	}

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
