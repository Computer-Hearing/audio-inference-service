package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

func Recovery(logger *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logger.Error("panic recovered",
						"error", err.(error).Error(),
						"stack", string(debug.Stack()),
						"path", r.URL.Path,
						"method", r.Method,
					)
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"error":"INTERNAL_ERROR"}`))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
