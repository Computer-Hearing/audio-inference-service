package handlers

import (
	"audio-inference-service/internal/chunks"
	"audio-inference-service/internal/domain"
	"audio-inference-service/internal/middleware"
	"audio-inference-service/internal/modules"
	"audio-inference-service/pkg"
	"os"

	"fmt"
	"log/slog"
	"net/http"
)

type Handlers struct {
	taskLoader modules.TaskManager
	catalog    modules.Catalog
	logger     *slog.Logger
}

func New(taskLoader modules.TaskManager, logger *slog.Logger, catalog modules.Catalog) *Handlers {
	return &Handlers{taskLoader: taskLoader, catalog: catalog, logger: logger}
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

	// Модель для инференса: заголовок X-Model, по умолчанию default-модель
	modelName := r.Header.Get(pkg.ModelHeaderKey)
	if modelName == "" {
		modelName = pkg.DefaultModelName
	}

	available, err := h.catalog.IsAvailable(r.Context(), modelName)
	if err != nil {
		// Тритон недоступен — разрешаем только дефолтную модель, остальное отклоняем
		if modelName != pkg.DefaultModelName {
			pkg.SendError(h.logger, w, fmt.Errorf("model catalog unavailable, cannot verify model"), http.StatusServiceUnavailable)
			return
		}
	} else if !available {
		pkg.SendError(h.logger, w, fmt.Errorf("unknown or unsupported model: %s", modelName), http.StatusBadRequest)
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
	if err := h.taskLoader.CreateTask(r.Context(), username, taskID, *ch, waves, modelName); err != nil {
		h.handleError(w, err)
		return
	}

	// Отдаем ответ
	pkg.SendJSON(h.logger, w, domain.TaskResponse{TaskID: taskID, Waves: waves, Model: modelName}, http.StatusCreated)
}

func (h *Handlers) ListModels(w http.ResponseWriter, r *http.Request) {
	models, err := h.catalog.List(r.Context())
	if err != nil {
		pkg.SendError(h.logger, w, fmt.Errorf("model catalog unavailable: %w", err), http.StatusServiceUnavailable)
		return
	}

	pkg.SendJSON(h.logger, w, models, http.StatusOK)
}

func (h *Handlers) GetTask(w http.ResponseWriter, r *http.Request) {
	username, ok := middleware.GetUsernameFromContext(r.Context())
	if !ok {
		pkg.SendError(h.logger, w, fmt.Errorf("username not found in context"), http.StatusUnauthorized)
		return
	}

	taskID := r.PathValue("taskID")
	if taskID == "" {
		pkg.SendError(h.logger, w, fmt.Errorf("task id is empty"), http.StatusBadRequest)
		return
	}

	task, err := h.taskLoader.GetTask(r.Context(), domain.Task(taskID), username)
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

func (h *Handlers) Register(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(pkg.UsernameCookieKey); err == nil {
		h.handleError(w, pkg.APIError{
			StatusCode: http.StatusConflict,
			Message:    "user already authenticated",
			Details: map[string]string{
				"username":    cookie.Value,
				"re-register": "empty cookie to re-register",
			},
		})
		return
	}

	var registerRequest struct {
		Username string `json:"username"`
	}

	if err := pkg.ParseJSONBody(r, &registerRequest); err != nil {
		h.handleError(w, err)
		return
	}

	username := pkg.UsernameGenerator(registerRequest.Username)
	h.logger.Info("Registering", "username", username)
	http.SetCookie(w, &http.Cookie{
		Name:     pkg.UsernameCookieKey,
		Value:    username,
		Path:     "/",
		MaxAge:   2147483647,
		HttpOnly: true,
		Secure:   os.Getenv("ENV") == "production",
		SameSite: http.SameSiteLaxMode,
	})

	w.WriteHeader(http.StatusCreated)
}

func (h *Handlers) handleError(w http.ResponseWriter, err error) {
	pkg.HandleError(w, h.logger, err)
}
