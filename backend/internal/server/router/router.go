package router

import (
	"audio-inference-service/internal/middleware"
	"audio-inference-service/internal/server/handlers"
	"log/slog"
	"net/http"
)

// New собирает HTTP-роутер: маршруты плюс мидлвары (recovery -> logging -> auth)
func New(logger *slog.Logger, h *handlers.Handlers) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/v1/tasks", h.CreateTask)
	mux.HandleFunc("GET /api/v1/tasks/{taskID}", h.GetTask)
	mux.HandleFunc("GET /api/v1/tasks/history", h.GetHistory)
	mux.HandleFunc("DELETE /api/v1/tasks/history", h.DeleteHistory)
	mux.HandleFunc("GET /api/v1/models", h.ListModels)

	return middleware.Recovery(logger)(
		middleware.Logging(logger)(
			middleware.CheckUsernameCookie(logger)(mux),
		),
	)
}
