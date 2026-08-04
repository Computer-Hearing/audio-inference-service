package main

import (
	"audio-inference-service/pkg"
	"log/slog"
	"os"
)

func init() {
	level := os.Getenv("LOG_LEVEL")
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: pkg.Loglevel(level), AddSource: true}))
	slog.SetDefault(logger)
}

func main() {

}
