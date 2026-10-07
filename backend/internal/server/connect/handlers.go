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
	"os"

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

func (h *Handlers) CreateTask(
	ctx context.Context, c *connect.Request[v1.CreateTaskRequest]) (
	*connect.Response[v1.CreateTaskResponse], error) {

	username, ok := GetUsernameFromContext(ctx)
	if !ok {
		return nil, pkg.ConnectError(pkg.NewUnauthorizedError("user is not authenticated"))
	}
	if err := username.IsValid(); err != nil {
		return nil, pkg.ConnectError(err)
	}

	available, err := h.catalog.IsAvailable(ctx, c.Msg.ModelName)
	if err != nil {
		// Тритон недоступен — отклоняем запрос
		return nil, pkg.ConnectError(
			pkg.NewServiceUnavailableWithDetails(
				"catalog: model catalog unavailable, cannot verify model",
				map[string]string{"error": err.Error()},
			),
		)
	} else if !available {
		// Модели нет в тритон
		return nil, pkg.ConnectError(
			pkg.NewBadRequestError(fmt.Sprintf("triton: unknown or unsupported model: %s", c.Msg.ModelName)),
		)
	}

	// генерируем таск id
	taskID := domain.GenerateTaskID(username.String())
	h.logger.Info("models", slog.Any("slice", h.modelStorage.Names()))
	modelConfig, ok := h.modelStorage.Get(c.Msg.ModelName)
	if !ok {
		return nil, pkg.ConnectError(
			pkg.NewBadRequestError(fmt.Sprintf("storage: unknown or unsupported model: %s", c.Msg.ModelName)),
		)
	}

	// получаем чанки
	ch, err := audio.Split(audio.Options{
		Data:            c.Msg.AudioFile,
		Filename:        c.Msg.Filename,
		SecondsPerChunk: modelConfig.SecondsPerChunk,
	})
	if err != nil {
		return nil, pkg.ConnectError(fmt.Errorf("audio split: %w", err))
	}

	payload := domain.AudioTaskPayload{ModelName: c.Msg.ModelName, Chunks: *ch}
	if err := h.taskLoader.CreateTask(ctx, username, taskID, payload); err != nil {
		return nil, pkg.ConnectError(fmt.Errorf("create task: %w", err))
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

func (h *Handlers) GetTask(
	ctx context.Context, c *connect.Request[v1.GetTaskRequest]) (
	*connect.Response[v1.GetTaskResponse], error) {

	username, ok := GetUsernameFromContext(ctx)
	if !ok {
		return nil, pkg.ConnectError(pkg.NewUnauthorizedError("user is not authenticated"))
	}
	if err := username.IsValid(); err != nil {
		return nil, pkg.ConnectError(err)
	}

	taskID := domain.TaskID(c.Msg.TaskId)
	if err := taskID.IsValid(); err != nil {
		return nil, pkg.ConnectError(err)
	}

	task, err := h.taskLoader.GetTask(ctx, taskID, username)
	if err != nil {
		return nil, pkg.ConnectError(fmt.Errorf("get task: %w", err))
	}

	return &connect.Response[v1.GetTaskResponse]{
		Msg: &v1.GetTaskResponse{
			Task: dto.Task2DTO(task),
		},
	}, nil
}

// Register регистрирует пользователя и выставляет username-cookie.
func (h *Handlers) Register(
	ctx context.Context, c *connect.Request[v1.RegisterRequest]) (
	*connect.Response[v1.RegisterResponse], error) {

	if existing, ok := cookieValue(c.Header(), pkg.UsernameCookieKey); ok {
		return nil, pkg.ConnectError(
			pkg.NewConflictError(fmt.Sprintf("user already authenticated: %s", existing)),
		)
	}

	username := pkg.UsernameGenerator(c.Msg.Username)
	h.logger.Debug("Generating username", "username", username)

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

func (h *Handlers) GetModels(
	ctx context.Context, _ *connect.Request[v1.GetModelsRequest]) (
	*connect.Response[v1.GetModelsResponse], error) {

	username, ok := GetUsernameFromContext(ctx)
	if !ok {
		return nil, pkg.ConnectError(pkg.NewUnauthorizedError("user is not authenticated"))
	}
	if err := username.IsValid(); err != nil {
		return nil, pkg.ConnectError(err)
	}

	// получаем модели из хранилища
	storageModels := h.modelStorage.GetAll()

	return connect.NewResponse(&v1.GetModelsResponse{
		Models: dto.Models2DTO(storageModels),
	}), nil
}
