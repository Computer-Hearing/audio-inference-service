package modules

import (
	"audio-inference-service/internal/chunks"
	"audio-inference-service/internal/domain"
	"audio-inference-service/pkg"
	"context"
)

// TaskManager - реализация должна уметь работать с задачами в бд, операции соответствуют методам
type TaskManager interface {
	// GetStatus - получить статус выполнения задачи
	GetStatus(ctx context.Context, taskID domain.Task) (pkg.TaskStatus, error)
	// GetHistory - получить всю историю запросов
	GetHistory(ctx context.Context, username domain.Username) ([]*chunks.FileInferenceResult, error)
	// DeleteHistory - очистить всю историю запросов
	DeleteHistory(ctx context.Context, username domain.Username) error

	// CreateTask - создать задачу, то есть создать в бд строку со статусом pending
	CreateTask(ctx context.Context, username domain.Username, taskID domain.Task, chunks chunks.AudioChunks, wave []float64) error

	// GetAndMarkProcessing берет строки из бд, меняет в них поле статус на processing или берет строки со статусом
	// processing если были. Они будут уже processing, если что-то упало (и не дошло до failure статуса)
	GetAndMarkProcessing(ctx context.Context, limit int) ([]domain.TaskPayload, error)

	// StatusSuccess и StatusFailure - cтатусы выполнения задач, успешно и с ошибкой соответственно
	StatusSuccess(ctx context.Context, taskID domain.Task, result *chunks.FileInferenceResult) error
	StatusFailure(ctx context.Context, taskID domain.Task) error

	// IncrementTaskError увеличивает счетчик ошибок.
	// Если лимит исчерпан, переводит задачу в 'failure', иначе возвращает в 'pending'
	IncrementTaskError(ctx context.Context, taskID domain.Task) error
}

// FilePredictor - реализация должна уметь брать задачу, выполнять и записывать в бд результат и статус
type FilePredictor interface {
	ProcessTask(ctx context.Context, job domain.TaskPayload) error
}
