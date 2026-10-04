package connect

import (
	"audio-inference-service/internal/domain"
	"context"
)

const UserContextKey = "username"

func GetUsernameFromContext(ctx context.Context) (domain.Username, bool) {
	username, ok := ctx.Value(UserContextKey).(string)
	return domain.Username(username), ok
}
