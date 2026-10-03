package sqlite

import (
	"audio-inference-service/internal/domain"
	"audio-inference-service/internal/domain/dto"
	"audio-inference-service/internal/modules/audio"
	"audio-inference-service/pkg"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
)

type SqliteTaskManager struct {
	db *sql.DB
}

func NewSQLiteTaskManager(db *sql.DB) *SqliteTaskManager {
	return &SqliteTaskManager{db: db}
}

func (m *SqliteTaskManager) GetTask(ctx context.Context, taskID domain.TaskID,
	username domain.Username) (*domain.Task, error) {
	if string(taskID) == "" {
		return nil, pkg.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "Validation failed",
			Details:    map[string]string{"taskID": "cannot be empty"},
		}
	}

	var (
		status     string
		resultJSON sql.NullString
		model      string
	)
	query := `SELECT status, result, model FROM tasks WHERE username = ? and id = ?`

	err := m.db.QueryRowContext(ctx, query, username, taskID).Scan(&status, &resultJSON, &model)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, pkg.APIError{
				StatusCode: http.StatusNotFound,
				Message:    "Task not found",
				Details:    map[string]string{"taskID": string(taskID)},
			}
		}
		return nil, pkg.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "Database error while getting task",
			Details:    map[string]string{"error": err.Error()},
		}
	}

	taskResult := &domain.Task{
		TaskID: taskID,
		Status: dto.TaskStatusFromString(status),
		Model:  model,
	}

	if resultJSON.Valid && resultJSON.String != "" {
		var res domain.FileInferenceResult
		if err := json.Unmarshal([]byte(resultJSON.String), &res); err != nil {
			return nil, pkg.APIError{
				StatusCode: http.StatusInternalServerError,
				Message:    "Failed to unmarshal task result",
				Details:    map[string]string{"taskID": string(taskID), "error": err.Error()},
			}
		}
		taskResult.Result = &res
	}

	return taskResult, nil
}

func (m *SqliteTaskManager) GetHistory(ctx context.Context, username domain.Username) ([]*domain.FileInferenceResult, error) {
	if string(username) == "" {
		return nil, pkg.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "Validation failed",
			Details:    map[string]string{"username": "cannot be empty"},
		}
	}

	query := `SELECT result FROM tasks WHERE username = ? AND status = ? ORDER BY created_at DESC`
	args := []any{username, dto.TaskStatusToString(domain.TaskStatusSuccess)}

	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, pkg.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "Database error while querying history",
			Details:    map[string]string{"error": err.Error()},
		}
	}
	defer rows.Close()

	var history []*domain.FileInferenceResult
	for rows.Next() {
		var resultJSON string
		if err := rows.Scan(&resultJSON); err != nil {
			return nil, pkg.APIError{
				StatusCode: http.StatusInternalServerError,
				Message:    "Database error while scanning history row",
				Details:    map[string]string{"error": err.Error()},
			}
		}

		var res domain.FileInferenceResult
		if err := json.Unmarshal([]byte(resultJSON), &res); err != nil {
			return nil, pkg.APIError{
				StatusCode: http.StatusInternalServerError,
				Message:    "Failed to unmarshal history result",
				Details:    map[string]string{"error": err.Error()},
			}
		}
		history = append(history, &res)
	}

	if err := rows.Err(); err != nil {
		return nil, pkg.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "Database error while iterating history rows",
			Details:    map[string]string{"error": err.Error()},
		}
	}

	return history, nil
}

func (m *SqliteTaskManager) DeleteHistory(ctx context.Context, username domain.Username) error {
	if string(username) == "" {
		return pkg.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "Validation failed",
			Details:    map[string]string{"username": "cannot be empty"},
		}
	}

	query := `DELETE FROM tasks WHERE username = ?`
	_, err := m.db.ExecContext(ctx, query, username)
	if err != nil {
		return pkg.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "Database error while deleting history",
			Details:    map[string]string{"error": err.Error()},
		}
	}

	return nil
}

func (m *SqliteTaskManager) CreateTask(
	ctx context.Context, username domain.Username,
	taskID domain.TaskID, payload domain.AudioTaskPayload) error {
	// Валидация всех полей
	details := make(map[string]string)
	if string(username) == "" {
		details["username"] = "cannot be empty"
	}
	if string(taskID) == "" {
		details["taskID"] = "cannot be empty"
	}
	if len(payload.Chunks.Layers) == 0 {
		details["chunks"] = "must contain at least one audio chunk"
	}
	if payload.ModelName == "" {
		details["model"] = "cannot be empty"
	}
	if len(details) > 0 {
		return pkg.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "Validation failed for task creation",
			Details:    details,
		}
	}

	chunksJSON, err := json.Marshal(payload.Chunks)
	if err != nil {
		return pkg.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to marshal audio chunks",
			Details:    map[string]string{"error": err.Error()},
		}
	}

	query := `
		INSERT INTO tasks (id, username, status, model, chunks, created_at, updated_at) 
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`

	_, err = m.db.ExecContext(ctx, query,
		taskID, username,
		dto.TaskStatusToString(domain.TaskStatusPending),
		payload.ModelName, string(chunksJSON),
	)
	if err != nil {
		return pkg.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "Database error while inserting task",
			Details:    map[string]string{"error": err.Error()},
		}
	}

	return nil
}

func (m *SqliteTaskManager) GetAndMarkProcessing(ctx context.Context, limit int) ([]domain.TaskPayload, error) {
	if limit <= 0 {
		return nil, pkg.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "Validation failed",
			Details:    map[string]string{"limit": "must be greater than 0"},
		}
	}

	query := `
		UPDATE tasks
		SET status = ?,
		    updated_at = CURRENT_TIMESTAMP,
		    retry_count = retry_count + 1
		WHERE id IN (
			SELECT id FROM tasks
			WHERE (status = ? 
			   OR (status = ? AND updated_at < datetime('now', '-10 minutes')))
			  AND retry_count < 3
			ORDER BY created_at ASC
			LIMIT ?
		)
		RETURNING id, chunks, model
	`

	rows, err := m.db.QueryContext(ctx, query,
		dto.TaskStatusToString(domain.TaskStatusProcessing),
		dto.TaskStatusToString(domain.TaskStatusPending),
		dto.TaskStatusToString(domain.TaskStatusProcessing),
		limit,
	)
	if err != nil {
		return nil, pkg.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "Database error while fetching tasks for processing",
			Details:    map[string]string{"error": err.Error()},
		}
	}
	defer rows.Close()

	var payloads []domain.TaskPayload
	for rows.Next() {
		var taskID, chunksJSON, model string

		if err := rows.Scan(&taskID, &chunksJSON, &model); err != nil {
			return nil, pkg.APIError{
				StatusCode: http.StatusInternalServerError,
				Message:    "Database error while scanning processing task payload",
				Details:    map[string]string{"error": err.Error()},
			}
		}

		var c audio.AudioChunks
		if err := json.Unmarshal([]byte(chunksJSON), &c); err != nil {
			// Локализуем ошибку: помечаем конкретную задачу битой и идем к следующей
			_ = m.IncrementTaskError(ctx, domain.TaskID(taskID))
			continue
		}

		payloads = append(payloads, domain.TaskPayload{
			TaskID: domain.TaskID(taskID),
			Payload: domain.AudioTaskPayload{
				ModelName: model,
				Chunks:    c,
			},
		})
	}

	return payloads, nil
}

func (m *SqliteTaskManager) StatusSuccess(ctx context.Context, taskID domain.TaskID, result *domain.FileInferenceResult) error {
	details := make(map[string]string)
	if string(taskID) == "" {
		details["taskID"] = "cannot be empty"
	}
	if result == nil {
		details["result"] = "cannot be nil"
	}

	if len(details) > 0 {
		return pkg.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "Validation failed",
			Details:    details,
		}
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return pkg.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to marshal result data",
			Details:    map[string]string{"error": err.Error()},
		}
	}

	query := `
		UPDATE tasks 
		SET status = ?, 
		    result = ?, 
		    updated_at = CURRENT_TIMESTAMP 
		WHERE id = ?
	`

	_, err = m.db.ExecContext(ctx, query, dto.TaskStatusToString(domain.TaskStatusSuccess), string(resultJSON), taskID)
	if err != nil {
		return pkg.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "Database error while setting status success",
			Details:    map[string]string{"error": err.Error()},
		}
	}

	return nil
}

func (m *SqliteTaskManager) StatusFailure(ctx context.Context, taskID domain.TaskID, result *domain.FileInferenceResult) error {
	if string(taskID) == "" {
		return pkg.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "Validation failed",
			Details:    map[string]string{"taskID": "cannot be empty"},
		}
	}

	var resultJSON interface{}
	if result != nil {
		raw, err := json.Marshal(result)
		if err != nil {
			return pkg.APIError{
				StatusCode: http.StatusInternalServerError,
				Message:    "Failed to marshal partial result data",
				Details:    map[string]string{"error": err.Error()},
			}
		}
		resultJSON = string(raw)
	}

	query := `
		UPDATE tasks 
		SET status = ?, 
		    result = ?, 
		    updated_at = CURRENT_TIMESTAMP 
		WHERE id = ?
	`

	_, err := m.db.ExecContext(ctx, query, dto.TaskStatusToString(domain.TaskStatusFailure), resultJSON, taskID)
	if err != nil {
		return pkg.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "Database error while setting status failure",
			Details:    map[string]string{"error": err.Error()},
		}
	}

	return nil
}

func (m *SqliteTaskManager) IncrementTaskError(ctx context.Context, taskID domain.TaskID) error {
	if string(taskID) == "" {
		return pkg.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "Validation failed",
			Details:    map[string]string{"taskID": "cannot be empty"},
		}
	}

	query := `
		UPDATE tasks 
		SET retry_count = retry_count + 1,
		    status = CASE 
		        WHEN retry_count >= 2 THEN ? 
		        ELSE ? 
		    END,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	_, err := m.db.ExecContext(ctx, query,
		dto.TaskStatusToString(domain.TaskStatusFailure),
		dto.TaskStatusToString(domain.TaskStatusPending),
		taskID,
	)
	if err != nil {
		return pkg.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "Database error while incrementing task error",
			Details:    map[string]string{"error": err.Error()},
		}
	}

	return nil
}

// ------------------------- РАБОТА С МОДЕЛЯМИ -------------------------------------------------------------------------

func (m *SqliteTaskManager) GetModels(ctx context.Context) (map[string]domain.Model, error) {
	query := `
		SELECT id, title, description, model_name, seconds_per_chunk
		FROM models
	`
	var models = make(map[string]domain.Model)

	rows, err := m.db.QueryContext(ctx, query)
	if err != nil {
		return nil, pkg.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "Database error while getting models",
			Details:    map[string]string{"error": err.Error()},
		}
	}
	for rows.Next() {
		var (
			id                            int64
			secondsPerChunk               uint8
			title, description, modelName string
		)

		err := rows.Scan(&id, &title, &description, &modelName, &secondsPerChunk)
		if err != nil {
			return nil, pkg.APIError{
				StatusCode: http.StatusInternalServerError,
				Message:    "Database error while scanning model",
				Details:    map[string]string{"error": err.Error()},
			}
		}

		models[modelName] = domain.Model{
			ID:              id,
			Title:           title,
			ModelName:       modelName,
			SecondsPerChunk: secondsPerChunk,
			Description:     description,
		}
	}

	return models, nil
}

func (m *SqliteTaskManager) DeleteModelByName(ctx context.Context, modelName string) error {
	if modelName == "" {
		return pkg.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "model name is required",
		}
	}

	query := `
		DELETE FROM models WHERE model_name = ?
	`

	_, err := m.db.ExecContext(ctx, query, modelName)
	if err != nil {
		return pkg.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "Database error while deleting model",
			Details:    map[string]string{"error": err.Error()},
		}
	}

	return nil
}

func (m *SqliteTaskManager) UpsertModel(ctx context.Context, model domain.Model) error {
	if err := model.Validate(); err != nil {
		return err
	}

	query := `
		INSERT INTO models (title, description, model_name, seconds_per_chunk)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (model_name) DO UPDATE 
		SET title = EXCLUDED.title,
			description = EXCLUDED.description,
			seconds_per_chunk = EXCLUDED.seconds_per_chunk,
		    id = EXCLUDED.id
	`

	_, err := m.db.ExecContext(ctx, query, model.Title, model.Description, model.ModelName, model.SecondsPerChunk)
	if err != nil {
		return pkg.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "Database error while upsert model",
			Details:    map[string]string{"error": err.Error()},
		}
	}
	return nil
}
