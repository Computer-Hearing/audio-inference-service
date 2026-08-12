package modules

import (
	"audio-inference-service/internal/domain"
	"audio-inference-service/internal/modules/catalog"
	"audio-inference-service/pkg"
	"context"
)

// TaskManager - реализация должна уметь работать с задачами в бд, операции соответствуют методам
type TaskManager[P, R any] interface {
	// GetTask - получить задачу: её статус и результат инференса, если он уже сохранён
	GetTask(ctx context.Context, taskID domain.Task, username domain.Username) (*domain.TaskResult[R], error)
	// GetHistory - получить всю историю запросов
	GetHistory(ctx context.Context, username domain.Username) ([]*R, error)
	// DeleteHistory - очистить всю историю запросов
	DeleteHistory(ctx context.Context, username domain.Username) error

	// CreateTask - создать задачу, то есть создать в бд строку со статусом pending
	CreateTask(ctx context.Context, username domain.Username, taskID domain.Task, payload P) error

	// GetAndMarkProcessing берет строки из бд, меняет в них поле статус на processing или берет строки со статусом
	// processing если были. Они будут уже processing, если что-то упало (и не дошло до failure статуса)
	GetAndMarkProcessing(ctx context.Context, limit int) ([]domain.TaskPayload[P], error)

	// StatusSuccess и StatusFailure - cтатусы выполнения задач, успешно и с ошибкой соответственно.
	// StatusFailure сохраняет частичный результат, если он успел собраться
	StatusSuccess(ctx context.Context, taskID domain.Task, result *R) error
	StatusFailure(ctx context.Context, taskID domain.Task, result *R) error

	// IncrementTaskError увеличивает счетчик ошибок.
	// Если лимит исчерпан, переводит задачу в 'failure', иначе возвращает в 'pending'
	IncrementTaskError(ctx context.Context, taskID domain.Task) error
}

// FilePredictor - реализация должна уметь брать задачу, выполнять и записывать в бд (с TaskManager работать) результат и статус
type FilePredictor[P any] interface {
	ProcessTask(ctx context.Context, job domain.TaskPayload[P]) error
}

type Catalog interface {
	// List возвращает модели, пригодные для вызова из сервиса под конкретный контракт входа
	List(ctx context.Context, contract pkg.InputContract) ([]catalog.ModelInfo, error)
	// IsAvailable проверяет, что модель выбрана правильно и доступна для использования
	IsAvailable(ctx context.Context, modelName string, contract pkg.InputContract) (bool, error)
}
