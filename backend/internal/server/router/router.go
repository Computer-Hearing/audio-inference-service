package router

import (
	"audio-inference-service/internal/middleware"
	"audio-inference-service/internal/server/handlers"
	"fmt"
	"log/slog"
	"net/http"
)

func New(logger *slog.Logger, h *handlers.Handlers, apiPrefix string) http.Handler {
	if logger == nil {
		panic("logger is nil")
	}
	if h == nil {
		panic("handler is nil")
	}
	if apiPrefix == "/" {
		apiPrefix = ""
	}

	mux := http.NewServeMux()

	mux.Handle(fmt.Sprintf("POST %s/api/v1/tasks", apiPrefix),
		middleware.CheckUsernameCookie(logger, http.HandlerFunc(h.CreateTask)))
	mux.Handle(fmt.Sprintf("GET %s/api/v1/tasks/{taskID}", apiPrefix),
		middleware.CheckUsernameCookie(logger, http.HandlerFunc(h.GetTask)))
	mux.Handle(fmt.Sprintf("GET %s/api/v1/tasks/history", apiPrefix),
		middleware.CheckUsernameCookie(logger, http.HandlerFunc(h.GetHistory)))
	mux.Handle(fmt.Sprintf("DELETE %s/api/v1/tasks/history", apiPrefix),
		middleware.CheckUsernameCookie(logger, http.HandlerFunc(h.DeleteHistory)))

	mux.Handle(fmt.Sprintf("GET %s/api/v1/models", apiPrefix),
		middleware.CheckUsernameCookie(logger, http.HandlerFunc(h.ListModels)))
	mux.HandleFunc(fmt.Sprintf("POST %s/api/v1/register", apiPrefix), h.Register)

	return middleware.Recovery(logger)(
		middleware.Logging(logger)(mux),
	)
}
