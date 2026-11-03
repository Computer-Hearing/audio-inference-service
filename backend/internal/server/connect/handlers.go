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
	taskID := domain.GenerateTaskID()

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
		h.logger.Error("split audio chunks", "error", err.Error)
		return nil, pkg.ConnectError(fmt.Errorf("audio split: %w", err))
	}
	h.logger.Debug("audio chunks splitted", "task_id", taskID.String(), "chunks len", (len(ch.Layers[0].Chunks) + len(ch.Layers[1].Chunks)))

	payload := domain.AudioTaskPayload{ModelName: c.Msg.ModelName, Chunks: *ch}
	if err := h.taskLoader.CreateTask(ctx, taskID, payload); err != nil {
		h.logger.Error("create task error", "task_id", taskID.String(), "error", err.Error())
		return nil, pkg.ConnectError(fmt.Errorf("create task: %w", err))
	}
	h.logger.Info("create task successful", "task_id", taskID.String())

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

	taskID := domain.TaskID(c.Msg.TaskId)
	if err := taskID.IsValid(); err != nil {
		return nil, pkg.ConnectError(err)
	}

	task, err := h.taskLoader.GetTask(ctx, taskID)
	if err != nil {
		return nil, pkg.ConnectError(fmt.Errorf("get task: %w", err))
	}

	return &connect.Response[v1.GetTaskResponse]{
		Msg: &v1.GetTaskResponse{
			Task: dto.Task2DTO(task),
		},
	}, nil
}

func (h *Handlers) GetModels(
	_ context.Context, _ *connect.Request[v1.GetModelsRequest]) (
	*connect.Response[v1.GetModelsResponse], error) {

	// получаем модели из хранилища
	storageModels := h.modelStorage.GetAll()

	return connect.NewResponse(&v1.GetModelsResponse{
		Models: dto.Models2DTO(storageModels),
	}), nil
}
