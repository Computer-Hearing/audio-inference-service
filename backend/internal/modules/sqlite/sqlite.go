package sqlite

import (
	"audio-inference-service/internal/chunks"
	"audio-inference-service/internal/domain"
	"audio-inference-service/pkg"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
)

type sqliteTaskManager struct {
	db *sql.DB
}

func NewSQLiteTaskManager(db *sql.DB) *sqliteTaskManager {
	return &sqliteTaskManager{db: db}
}

func (m *sqliteTaskManager) GetStatus(ctx context.Context, taskID domain.Task) (pkg.TaskStatus, error) {
	if string(taskID) == "" {
		return "", pkg.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "Validation failed",
			Details:    map[string]string{"taskID": "cannot be empty"},
		}
	}

	var status string
	query := `SELECT status FROM tasks WHERE id = ?`

	err := m.db.QueryRowContext(ctx, query, taskID).Scan(&status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", pkg.APIError{
				StatusCode: http.StatusNotFound,
				Message:    "Task not found",
				Details:    map[string]string{"taskID": string(taskID)},
			}
		}
		return "", pkg.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "Database error while getting task status",
			Details:    map[string]string{"error": err.Error()},
		}
	}

	return pkg.TaskStatus(status), nil
}

func (m *sqliteTaskManager) GetHistory(ctx context.Context, username domain.Username) ([]*chunks.FileInferenceResult, error) {
	if string(username) == "" {
		return nil, pkg.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "Validation failed",
			Details:    map[string]string{"username": "cannot be empty"},
		}
	}

	query := `SELECT result FROM tasks WHERE username = ? AND status = 'success' ORDER BY created_at DESC`

	rows, err := m.db.QueryContext(ctx, query, username)
	if err != nil {
		return nil, pkg.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "Database error while querying history",
			Details:    map[string]string{"error": err.Error()},
		}
	}
	defer rows.Close()

	var history []*chunks.FileInferenceResult
	for rows.Next() {
		var resultJSON string
		if err := rows.Scan(&resultJSON); err != nil {
			return nil, pkg.APIError{
				StatusCode: http.StatusInternalServerError,
				Message:    "Database error while scanning history row",
				Details:    map[string]string{"error": err.Error()},
			}
		}

		var res chunks.FileInferenceResult
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

func (m *sqliteTaskManager) DeleteHistory(ctx context.Context, username domain.Username) error {
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

func (m *sqliteTaskManager) CreateTask(
	ctx context.Context, username domain.Username,
	taskID domain.Task, audioChunks chunks.AudioChunks, wave []float64) error {
	// Валидация всех полей
	details := make(map[string]string)
	if string(username) == "" {
		details["username"] = "cannot be empty"
	}
	if string(taskID) == "" {
		details["taskID"] = "cannot be empty"
	}
	if len(audioChunks.Chunks) == 0 {
		details["chunks"] = "must contain at least one audio chunk"
	}
	if len(wave) == 0 {
		// Спектрограмму пока что пустой пропускаем, если есть, посмотрим потом.
		// details["wave"] = "cannot be empty"
	}

	if len(details) > 0 {
		return pkg.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "Validation failed for task creation",
			Details:    details,
		}
	}

	chunksJSON, err := json.Marshal(audioChunks)
	if err != nil {
		return pkg.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to marshal audio chunks",
			Details:    map[string]string{"error": err.Error()},
		}
	}

	waveJSON, err := json.Marshal(wave)
	if err != nil {
		return pkg.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to marshal wave data",
			Details:    map[string]string{"error": err.Error()},
		}
	}

	query := `
		INSERT INTO tasks (id, username, status, chunks, wave, created_at, updated_at) 
		VALUES (?, ?, 'pending', ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`

	_, err = m.db.ExecContext(ctx, query, taskID, username, string(chunksJSON), string(waveJSON))
	if err != nil {
		return pkg.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "Database error while inserting task",
			Details:    map[string]string{"error": err.Error()},
		}
	}

	return nil
}

func (m *sqliteTaskManager) GetAndMarkProcessing(ctx context.Context, limit int) ([]domain.TaskPayload, error) {
	if limit <= 0 {
		return nil, pkg.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "Validation failed",
			Details:    map[string]string{"limit": "must be greater than 0"},
		}
	}

	query := `
		UPDATE tasks
		SET status = 'processing',
		    updated_at = CURRENT_TIMESTAMP,
		    retry_count = retry_count + 1
		WHERE id IN (
			SELECT id FROM tasks
			WHERE (status = 'pending' 
			   OR (status = 'processing' AND updated_at < datetime('now', '-10 minutes')))
			  AND retry_count < 3
			ORDER BY created_at ASC
			LIMIT ?
		)
		RETURNING id, chunks
	`

	rows, err := m.db.QueryContext(ctx, query, limit)
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
		var taskID, chunksJSON string

		if err := rows.Scan(&taskID, &chunksJSON); err != nil {
			return nil, pkg.APIError{
				StatusCode: http.StatusInternalServerError,
				Message:    "Database error while scanning processing task payload",
				Details:    map[string]string{"error": err.Error()},
			}
		}

		var c chunks.AudioChunks
		if err := json.Unmarshal([]byte(chunksJSON), &c); err != nil {
			// Локализуем ошибку: помечаем конкретную задачу битой и идем к следующей
			_ = m.IncrementTaskError(ctx, domain.Task(taskID))
			continue
		}

		payloads = append(payloads, domain.TaskPayload{
			TaskID: domain.Task(taskID),
			Chunks: c,
		})
	}

	return payloads, nil
}

func (m *sqliteTaskManager) StatusSuccess(ctx context.Context, taskID domain.Task, result *chunks.FileInferenceResult) error {
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
		SET status = 'success', 
		    result = ?, 
		    updated_at = CURRENT_TIMESTAMP 
		WHERE id = ?
	`

	_, err = m.db.ExecContext(ctx, query, string(resultJSON), taskID)
	if err != nil {
		return pkg.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "Database error while setting status success",
			Details:    map[string]string{"error": err.Error()},
		}
	}

	return nil
}

func (m *sqliteTaskManager) StatusFailure(ctx context.Context, taskID domain.Task) error {
	if string(taskID) == "" {
		return pkg.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "Validation failed",
			Details:    map[string]string{"taskID": "cannot be empty"},
		}
	}

	query := `
		UPDATE tasks 
		SET status = 'failure', 
		    updated_at = CURRENT_TIMESTAMP 
		WHERE id = ?
	`

	_, err := m.db.ExecContext(ctx, query, taskID)
	if err != nil {
		return pkg.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "Database error while setting status failure",
			Details:    map[string]string{"error": err.Error()},
		}
	}

	return nil
}

func (m *sqliteTaskManager) IncrementTaskError(ctx context.Context, taskID domain.Task) error {
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
		        WHEN retry_count >= 2 THEN 'failure' 
		        ELSE 'pending' 
		    END,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	_, err := m.db.ExecContext(ctx, query, taskID)
	if err != nil {
		return pkg.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "Database error while incrementing task error",
			Details:    map[string]string{"error": err.Error()},
		}
	}

	return nil
}
