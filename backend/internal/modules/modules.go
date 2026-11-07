package modules

import (
	"audio-inference-service/internal/domain"
	"audio-inference-service/internal/modules/catalog"
	"context"
	"time"
)

// TaskManager - хранилище состояния задач. Реализация может быть на Redis,
// Postgres или в памяти: интерфейс не привязан к конкретному бэкенду.
type TaskManager interface {
	// GetTask - получить задачу: её статус и результат инференса, если он уже сохранён
	GetTask(ctx context.Context, taskID domain.TaskID) (*domain.Task, error)

	// CreateTask - сохранить задачу со статусом pending и поставить её в очередь
	CreateTask(ctx context.Context, taskID domain.TaskID, payload domain.AudioTaskPayload) error

	// GetPayload - получить данные задачи для обработки воркером.
	// Второе значение — false, если задача уже истекла или завершилась.
	GetPayload(ctx context.Context, taskID domain.TaskID) (*domain.AudioTaskPayload, bool, error)

	// MarkProcessing переводит задачу в processing и увеличивает счётчик попыток.
	// Второе значение — false, если задачи уже нет.
	MarkProcessing(ctx context.Context, taskID domain.TaskID) (attempts int, found bool, err error)

	// StatusSuccess и StatusFailure - cтатусы выполнения задач, успешно и с ошибкой соответственно.
	// StatusFailure сохраняет частичный результат, если он успел собраться
	StatusSuccess(ctx context.Context, taskID domain.TaskID, result *domain.FileInferenceResult) error
	StatusFailure(ctx context.Context, taskID domain.TaskID, result *domain.FileInferenceResult) error
}

// TaskQueue - доставка задач воркерам. Реализация может быть на Redis Streams,
// NATS JetStream и т.п.
type TaskQueue interface {
	// EnsureGroup создаёт consumer group, если её ещё нет
	EnsureGroup(ctx context.Context) error

	// Read забирает до count новых сообщений. Пустой срез — задач нет.
	Read(ctx context.Context, count int, block time.Duration) ([]domain.TaskDelivery, error)

	// Reclaim забирает сообщения, не подтверждённые дольше minIdle.
	Reclaim(ctx context.Context, minIdle time.Duration, count int) ([]domain.TaskDelivery, error)

	// Ack подтверждает обработку сообщения.
	Ack(ctx context.Context, receipt string) error
}

// FilePredictor - реализация должна уметь брать задачу, выполнять и записывать в бд (с TaskManager работать) результат и статус
type FilePredictor interface {
	ProcessTask(ctx context.Context, job domain.TaskPayload) error
}

type Catalog interface {
	// List возвращает модели, пригодные для вызова из сервиса под аудио-контракт
	List(ctx context.Context) ([]catalog.ModelInfo, error)
	// IsAvailable проверяет, что модель выбрана правильно и доступна для использования
	IsAvailable(ctx context.Context, modelName string) (bool, error)
}
