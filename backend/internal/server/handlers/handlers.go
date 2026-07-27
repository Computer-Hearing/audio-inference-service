package handlers

import (
	"audio-inference-service/internal/service"
	"audio-inference-service/pkg"
	"audio-inference-service/pkg/chunks"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
)

type Handlers struct {
	predictor  service.FilePredictor
	taskLoader service.TaskLoader
	logger     *slog.Logger
}

func (h *Handlers) CreateTask(w http.ResponseWriter, r *http.Request) {
	userName, err := r.Cookie(pkg.UsernameCookieKey)
	if err != nil {
		pkg.SendError(h.logger, w, fmt.Errorf("username cookie is empty"), http.StatusUnauthorized)
		return
	}

	// Проверяем есть ли имя пользователя
	userDomain := pkg.Username(userName.Value)
	if err := userDomain.IsValid(); err != nil {
		h.handleError(w, err)
		return
	}
	// генерируем таску
	taskID := pkg.GenerateTaskID(userName.Value)
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
	if err := h.taskLoader.CreateTask(r.Context(), userDomain, taskID, ch, waves); err != nil {
		h.handleError(w, err)
		return
	}

	// Отдаем ответ
	pkg.SendJSON(h.logger, w, pkg.TaskResponse{TaskID: taskID, Waves: waves}, http.StatusCreated)
}

func (h *Handlers) handleError(w http.ResponseWriter, err error) {
	var errApi pkg.APIError
	if errors.As(err, &errApi) {
		pkg.SendError(h.logger, w, &errApi, errApi.StatusCode)
		return
	}
	pkg.SendError(h.logger, w, err, http.StatusInternalServerError)
}
