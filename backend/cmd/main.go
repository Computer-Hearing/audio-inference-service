package main

import (
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

func init() {
	level := os.Getenv("LOG_LEVEL")
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: pkg.GetLoglevel(level), AddSource: true}))
	slog.SetDefault(logger)
}

func main() {
	logger := slog.Default()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	dbPath := envOrDefault("DB_PATH", "sqlite.db")
	db, err := pkg.SqliteOpen(dbPath, nil)
	if err != nil {
		logger.Error("failed to open sqlite database", "err", err.Error())
		os.Exit(1)
	}
	defer db.Close()
	logger.Info("sqlite database opened", "db_path", dbPath)

	tritonAddr := envOrDefault("TRITON_ADDR", "192.168.3.17:8001")
	tritonClient, err := triton.NewTritonClient(triton.DefaultConfig(tritonAddr))
	if err != nil {
		logger.Error("failed to create triton client", "err", err.Error())
		os.Exit(1)
	}
	defer tritonClient.Close()

	if err := tritonClient.Connect(ctx); err != nil {
		logger.Error("failed to connect to triton", "addr", tritonAddr, "err", err.Error())
		os.Exit(1)
	}
	logger.Info("connected to triton", "addr", tritonAddr)

	taskManager := sqlite.NewSQLiteTaskManager(db)
	predict := &predictor.Predictor{
		TritonConnector: tritonClient,
		TaskManager:     taskManager,
	}

	// Запускаем воркеры и диспетчера задач
	taskpipe.StartPipeline(ctx, taskManager, predict)

	handlers := handlers.New(taskManager, logger)
	httpAddr := envOrDefault("HTTP_ADDR", ":8080")
	srv := &http.Server{
		Addr:         httpAddr,
		Handler:      router.New(logger, handlers),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 5 * time.Minute,
	}

	go func() {
		logger.Info("http server started", "addr", httpAddr)
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

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
