package connect

import (
	"audio-inference-service/internal/domain"
	"audio-inference-service/pkg"
	"context"
	"net/http"

	"connectrpc.com/connect"
)

// NewUsernameInterceptor оборачивает unary-вызов, добавляя username в контекст
func NewUsernameInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if val, ok := cookieValue(req.Header(), pkg.UsernameCookieKey); ok {
				ctx = context.WithValue(ctx, userCtxKey, domain.Username(val))
			}
			return next(ctx, req)
		}
	}
}

// cookieValue извлекает значение cookie по имени из заголовков.
func cookieValue(h http.Header, name string) (string, bool) {
	r := http.Request{Header: h}
	for _, c := range r.Cookies() {
		if c.Name == name {
			return c.Value, true
		}
	}
	return "", false
}

// ctxKey тип ключа контекста, чтобы избежать коллизий со строковыми ключами.
type ctxKey int

const userCtxKey ctxKey = iota

// GetUsernameFromContext возвращает username из контекста.
func GetUsernameFromContext(ctx context.Context) (domain.Username, bool) {
	username, ok := ctx.Value(userCtxKey).(domain.Username)
	return username, ok
}
