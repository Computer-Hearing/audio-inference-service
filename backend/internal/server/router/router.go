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

	apiPrefix = fixApiPrefix(apiPrefix)

	mux := http.NewServeMux()

	mux.Handle(fmt.Sprintf("POST %s/api/v1/tasks", apiPrefix),
		middleware.CheckUsernameCookie(logger, http.HandlerFunc(h.CreateTask)))
	mux.Handle(fmt.Sprintf("GET %s/api/v1/tasks/{taskID}", apiPrefix),
		middleware.CheckUsernameCookie(logger, http.HandlerFunc(h.GetTask)))
	mux.Handle(fmt.Sprintf("GET %s/api/v1/tasks/history", apiPrefix),
		middleware.CheckUsernameCookie(logger, http.HandlerFunc(h.GetHistory)))
	mux.Handle(fmt.Sprintf("DELETE %s/api/v1/tasks/history", apiPrefix),
		middleware.CheckUsernameCookie(logger, http.HandlerFunc(h.DeleteHistory)))

	mux.HandleFunc(fmt.Sprintf("POST %s/api/v1/register", apiPrefix), h.Register)

	mux.Handle(fmt.Sprintf("DELETE %s/api/v1/model/{model_name}", apiPrefix),
		middleware.CheckUsernameCookie(logger, http.HandlerFunc(h.DeleteModel)))
	mux.Handle(fmt.Sprintf("POST %s/api/v1/model", apiPrefix),
		middleware.CheckUsernameCookie(logger, http.HandlerFunc(h.UpsertModel)))
	mux.Handle(fmt.Sprintf("GET %s/api/v1/models", apiPrefix),
		middleware.CheckUsernameCookie(logger, http.HandlerFunc(h.ListModels)))

	return middleware.Recovery(logger)(
		middleware.Logging(logger)(mux),
	)
}

func fixApiPrefix(apiPrefix string) string {
	fixedApiPrefix := apiPrefix

	if apiPrefix == "/" {
		return ""
	}
	if len(apiPrefix) > 0 && apiPrefix[0] != '/' {
		fixedApiPrefix = "/" + fixedApiPrefix
	}
	if len(apiPrefix) > 0 && apiPrefix[len(apiPrefix)-1] == '/' {
		fixedApiPrefix = fixedApiPrefix[:len(fixedApiPrefix)-1]
	}

	return fixedApiPrefix
}
