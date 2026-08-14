package router

import (
	"audio-inference-service/internal/middleware"
	"audio-inference-service/internal/server/handlers"
	"log/slog"
	"net/http"
)

func New(logger *slog.Logger, h *handlers.Handlers) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("POST /api/v1/tasks", middleware.CheckUsernameCookie(logger, http.HandlerFunc(h.CreateTask)))
	mux.Handle("GET /api/v1/tasks/{taskID}", middleware.CheckUsernameCookie(logger, http.HandlerFunc(h.GetTask)))

	mux.Handle("GET /api/v1/tasks/history", middleware.CheckUsernameCookie(logger, http.HandlerFunc(h.GetHistory)))
	mux.Handle("DELETE /api/v1/tasks/history", middleware.CheckUsernameCookie(logger, http.HandlerFunc(h.DeleteHistory)))

	mux.Handle("GET /api/v1/models", middleware.CheckUsernameCookie(logger, http.HandlerFunc(h.ListModels)))
	mux.HandleFunc("POST /api/v1/register", h.Register)

	return middleware.Recovery(logger)(
		middleware.Logging(logger)(mux),
	)
}
