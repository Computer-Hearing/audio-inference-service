package connect

import (
	v1 "audio-inference-service/gen/inference/v1"
	"audio-inference-service/gen/inference/v1/inferencev1connect"
	"audio-inference-service/internal/domain"
	"audio-inference-service/internal/modules"
	"audio-inference-service/internal/modules/audio"
	"audio-inference-service/pkg"
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
)

type Handlers struct {
	taskLoader modules.TaskManager
	catalog    modules.Catalog
	logger     *slog.Logger
}

type Options struct {
	TaskLoader modules.TaskManager
	Catalog    modules.Catalog
	Logger     *slog.Logger
}

func New(opts Options) *Handlers {
	return &Handlers{taskLoader: opts.TaskLoader, catalog: opts.Catalog, logger: opts.Logger}
}

var _ inferencev1connect.InferenceServiceClient = (*Handlers)(nil)

func (h Handlers) CreateTask(
	ctx context.Context, c *connect.Request[v1.CreateTaskRequest]) (
	*connect.Response[v1.CreateTaskResponse], error) {

	username, ok := GetUsernameFromContext(ctx)
	if !ok {
		return nil, pkg.APIError{
			StatusCode: http.StatusUnauthorized,
			Message:    "user is not authenticated",
		}
	}
	if err := username.IsValid(); err != nil {
		return nil, err
	}

	available, err := h.catalog.IsAvailable(ctx, c.Msg.ModelName)
	if err != nil {
		// Тритон недоступен — отклоняем запрос
		return nil, pkg.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "model catalog unavailable, cannot verify model",
			Details:    map[string]string{"error": err.Error()},
		}

	} else if !available {
		// Модели нет в тритон
		return nil, pkg.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("unknown or unsupported model: %s", c.Msg.ModelName),
		}
	}

	// генерируем таск id
	taskID := domain.GenerateTaskID(username.String())

	// получаем чанки
	ch, err := audio.Split(audio.Options{
		Data:     c.Msg.AudioFile,
		Filename: c.Msg.Filename,
	})
	if err != nil {
		h.handleError(w, err)
		return
	}
}

func (h Handlers) GetTask(
	ctx context.Context, c *connect.Request[v1.GetTaskRequest]) (
	*connect.Response[v1.GetTaskResponse], error) {
	//TODO implement me
	panic("implement me")
}
