package connect

import (
	v1 "audio-inference-service/gen/inference/v1"
	"audio-inference-service/pkg"
	"context"
	"fmt"
	"net/http"
	"os"

	"connectrpc.com/connect"
)

// Register регистрирует пользователя и выставляет username-cookie.
func (h Handlers) Register(
	ctx context.Context, c *connect.Request[v1.RegisterRequest]) (
	*connect.Response[v1.RegisterResponse], error) {

	if existing, ok := cookieValue(c.Header(), pkg.UsernameCookieKey); ok {
		return nil, connect.NewError(connect.CodeAlreadyExists, errAlreadyAuthenticated(existing))
	}

	username := pkg.UsernameGenerator(c.Msg.Username)
	h.logger.Info("Registering", "username", username)

	cookie := &http.Cookie{
		Name:     pkg.UsernameCookieKey,
		Value:    username,
		Path:     "/",
		MaxAge:   2147483647,
		HttpOnly: true,
		Secure:   os.Getenv("ENV") == "production",
		SameSite: http.SameSiteLaxMode,
	}

	resp := connect.NewResponse(&v1.RegisterResponse{Username: username})
	resp.Header().Add("Set-Cookie", cookie.String())

	return resp, nil
}

// errAlreadyAuthenticated возвращает ошибку для случая, когда cookie уже есть.
func errAlreadyAuthenticated(username string) error {
	return fmt.Errorf("user already authenticated: %s", username)
}
