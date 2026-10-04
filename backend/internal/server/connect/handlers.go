package connect

import (
	v1 "audio-inference-service/gen/inference/v1"
	"audio-inference-service/gen/inference/v1/inferencev1connect"
	"audio-inference-service/internal/domain"
	"audio-inference-service/internal/modules"
	"audio-inference-service/internal/modules/audio"
	"audio-inference-service/internal/modules/models"
	"audio-inference-service/internal/server/connect/dto"
	"audio-inference-service/pkg"
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
)

type Handlers struct {
	taskLoader   modules.TaskManager
	catalog      modules.Catalog
	modelStorage *models.Storage
	logger       *slog.Logger
}

type Options struct {
	TaskLoader   modules.TaskManager
	Catalog      modules.Catalog
	ModelStorage *models.Storage
	Logger       *slog.Logger
}

func New(opts *Options) *Handlers {
	return &Handlers{taskLoader: opts.TaskLoader, catalog: opts.Catalog, logger: opts.Logger, modelStorage: opts.ModelStorage}
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
			Message:    fmt.Sprintf("triton: unknown or unsupported model: %s", c.Msg.ModelName),
		}
	}

	// генерируем таск id
	taskID := domain.GenerateTaskID(username.String())
	h.logger.Info("models", slog.Any("slice", h.modelStorage.Names()))
	modelConfig, ok := h.modelStorage.Get(c.Msg.ModelName)
	if !ok {
		return nil, pkg.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("storage: unknown or unsupported model: %s", c.Msg.ModelName),
		}
	}

	// получаем чанки
	ch, err := audio.Split(audio.Options{
		Data:            c.Msg.AudioFile,
		Filename:        c.Msg.Filename,
		SecondsPerChunk: modelConfig.SecondsPerChunk,
	})
	if err != nil {
		return nil, err
	}

	payload := domain.AudioTaskPayload{ModelName: c.Msg.ModelName, Chunks: *ch}
	if err := h.taskLoader.CreateTask(ctx, username, taskID, payload); err != nil {
		return nil, err
	}

	// Отдаем ответ
	return &connect.Response[v1.CreateTaskResponse]{
		Msg: &v1.CreateTaskResponse{
			Task: dto.Task2DTO(&domain.Task{
				TaskID: taskID,
				Model:  c.Msg.ModelName,
				Status: domain.TaskStatusPending,
			}),
		},
	}, nil
}

func (h Handlers) GetTask(
	ctx context.Context, c *connect.Request[v1.GetTaskRequest]) (
	*connect.Response[v1.GetTaskResponse], error) {

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

	taskID := domain.TaskID(c.Msg.TaskId)
	if err := taskID.IsValid(); err != nil {
		return nil, err
	}

	task, err := h.taskLoader.GetTask(ctx, taskID, username)
	if err != nil {
		return nil, err
	}

	return &connect.Response[v1.GetTaskResponse]{
		Msg: &v1.GetTaskResponse{
			Task: dto.Task2DTO(task),
		},
	}, nil
}
