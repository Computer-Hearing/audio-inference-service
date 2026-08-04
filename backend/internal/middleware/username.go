package middleware

import (
	"audio-inference-service/internal/domain"
	"audio-inference-service/pkg"
	"context"
	"log/slog"
	"net/http"
)

const UserContextKey = "username"

func CheckUsernameCookie(logger *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(pkg.UsernameCookieKey)
			if err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`user is unauthorized`))
				logger.Warn("user is unauthorized", "ip", r.RemoteAddr, "userAgent", r.UserAgent())
				return
			}

			userGSSC := cookie.Value
			//TODO сделать GSSC
			username := userGSSC
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), UserContextKey, username)))
		})
	}
}

func GetUsernameFromContext(ctx context.Context) (domain.Username, bool) {
	username, ok := ctx.Value(UserContextKey).(string)
	return domain.Username(username), ok
}
