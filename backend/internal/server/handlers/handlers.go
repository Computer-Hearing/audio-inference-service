package handlers

import (
	"audio-inference-service/internal/chunks"
	"audio-inference-service/internal/domain"
	"audio-inference-service/internal/middleware"
	"audio-inference-service/internal/modules"
	"audio-inference-service/pkg"

	"errors"
	"fmt"
	"log/slog"
	"net/http"
)

type Handlers struct {
	taskLoader modules.TaskManager
	logger     *slog.Logger
}

func New(taskLoader modules.TaskManager, logger *slog.Logger) *Handlers {
	return &Handlers{taskLoader: taskLoader, logger: logger}
}

func (h *Handlers) CreateTask(w http.ResponseWriter, r *http.Request) {
	username, ok := middleware.GetUsernameFromContext(r.Context())
	if !ok {
		pkg.SendError(h.logger, w, fmt.Errorf("username cookie/header are empty"), http.StatusUnauthorized)
		return
	}

	if err := username.IsValid(); err != nil {
		h.handleError(w, err)
		return
	}
	// генерируем таску
	taskID := domain.GenerateTaskID(username.String())
	// получаем чанки - раздробленный звуковой файл на несколько по две секунды, переведенные в байты
	ch, err := chunks.ChunksFromRequest(r)
	if err != nil {
		h.handleError(w, err)
		return
	}
	// Получаем спектрограмму звуковую, просто массив флоатов, то есть для нас это столбики, чтобы красиво отрисовать звук
	waves, err := chunks.AudioWaveFromRequest(r)
	if err != nil {
		h.handleError(w, err)
		return
	}

	// Создаем таску
	if err := h.taskLoader.CreateTask(r.Context(), username, taskID, *ch, waves); err != nil {
		h.handleError(w, err)
		return
	}

	// Отдаем ответ
	pkg.SendJSON(h.logger, w, domain.TaskResponse{TaskID: taskID, Waves: waves}, http.StatusCreated)
}

func (h *Handlers) GetTask(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("taskID")
	if taskID == "" {
		pkg.SendError(h.logger, w, fmt.Errorf("task id is empty"), http.StatusBadRequest)
		return
	}

	task, err := h.taskLoader.GetTask(r.Context(), domain.Task(taskID))
	if err != nil {
		h.handleError(w, err)
		return
	}

	pkg.SendJSON(h.logger, w, task, http.StatusOK)
}

func (h *Handlers) GetHistory(w http.ResponseWriter, r *http.Request) {
	username, ok := middleware.GetUsernameFromContext(r.Context())
	if !ok {
		pkg.SendError(h.logger, w, fmt.Errorf("username not found in context"), http.StatusUnauthorized)
		return
	}

	history, err := h.taskLoader.GetHistory(r.Context(), username)
	if err != nil {
		h.handleError(w, err)
		return
	}

	pkg.SendJSON(h.logger, w, history, http.StatusOK)
}

func (h *Handlers) DeleteHistory(w http.ResponseWriter, r *http.Request) {
	username, ok := middleware.GetUsernameFromContext(r.Context())
	if !ok {
		pkg.SendError(h.logger, w, fmt.Errorf("username not found in context"), http.StatusUnauthorized)
		return
	}

	if err := h.taskLoader.DeleteHistory(r.Context(), username); err != nil {
		h.handleError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) handleError(w http.ResponseWriter, err error) {
	var errApi pkg.APIError
	if errors.As(err, &errApi) {
		pkg.SendError(h.logger, w, &errApi, errApi.StatusCode)
		return
	}
	pkg.SendError(h.logger, w, err, http.StatusInternalServerError)
}
