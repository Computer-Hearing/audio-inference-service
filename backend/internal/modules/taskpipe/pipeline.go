package taskpipe

import (
	"audio-inference-service/internal/domain"
	"audio-inference-service/internal/modules"
	"context"
	"log/slog"
	"time"
)

const (
	taskCountLimit   = 10
	taskChanCapacity = 20
	workersCount     = 5

	// readBlock — сколько ждать новых сообщений в очереди за один вызов.
	readBlock = 2 * time.Second

	// minReclaimEvery — минимальный интервал проверки зависших сообщений.
	minReclaimEvery = 30 * time.Second
)

// Options параметры пайплайна обработки задач.
type Options struct {
	Logger        *slog.Logger
	Store         modules.TaskManager
	Queue         modules.TaskQueue
	Predictor     modules.FilePredictor
	AckWait       time.Duration
	MaxDeliveries int
}

// StartPipeline запускает воркеров, читателя очереди и восстановление
// сообщений, которые не были подтверждены (упавший воркер/рестарт).
func StartPipeline(ctx context.Context, opts Options) {
	jobsChan := make(chan domain.TaskDelivery, taskChanCapacity)

	for i := 0; i < workersCount; i++ {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case delivery := <-jobsChan:
					processDelivery(ctx, opts, delivery)
				}
			}
		}()
	}

	// Читатель очереди: XREADGROUP вместо опроса БД раз в 2 секунды.
	go func() {
		for {
			deliveries, err := opts.Queue.Read(ctx, taskCountLimit, readBlock)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				opts.Logger.Error("read task queue", "err", err.Error())
				time.Sleep(time.Second)
				continue
			}

			for _, delivery := range deliveries {
				select {
				case <-ctx.Done():
					return
				case jobsChan <- delivery:
				}
			}
		}
	}()

	// Восстановление сообщений, оставшихся в PEL после падения воркера.
	go reclaimLoop(ctx, opts, jobsChan)
}

func reclaimLoop(ctx context.Context, opts Options, jobsChan chan<- domain.TaskDelivery) {
	interval := opts.AckWait / 2
	if interval < minReclaimEvery {
		interval = minReclaimEvery
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			deliveries, err := opts.Queue.Reclaim(ctx, opts.AckWait, taskCountLimit)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				opts.Logger.Error("reclaim tasks", "err", err.Error())
				continue
			}

			for _, delivery := range deliveries {
				select {
				case <-ctx.Done():
					return
				case jobsChan <- delivery:
				}
			}
		}
	}
}

func processDelivery(ctx context.Context, opts Options, delivery domain.TaskDelivery) {
	logger := opts.Logger.With("task_id", delivery.TaskID.String())

	attempts, found, err := opts.Store.MarkProcessing(ctx, delivery.TaskID)
	if err != nil {
		// Redis/сеть: не подтверждаем сообщение, оно вернётся через Reclaim.
		logger.Error("mark task processing", "err", err.Error())
		return
	}
	if !found {
		logger.Warn("task is already finished or expired")
		ack(ctx, opts, logger, delivery)
		return
	}

	if attempts > opts.MaxDeliveries {
		logger.Warn("task delivery limit exceeded", "attempts", attempts)
		if err := opts.Store.StatusFailure(ctx, delivery.TaskID, nil); err != nil {
			logger.Error("set failure status", "err", err.Error())
			return
		}
		ack(ctx, opts, logger, delivery)
		return
	}

	payload, found, err := opts.Store.GetPayload(ctx, delivery.TaskID)
	if err != nil {
		logger.Error("get task payload", "err", err.Error())
		return
	}
	if !found {
		logger.Warn("task payload expired")
		_ = opts.Store.StatusFailure(ctx, delivery.TaskID, nil)
		ack(ctx, opts, logger, delivery)
		return
	}

	logger.Debug("process task")
	if err := opts.Predictor.ProcessTask(ctx, domain.TaskPayload{
		TaskID:  delivery.TaskID,
		Payload: *payload,
	}); err != nil {
		logger.Error("failed to process task", "err", err.Error())

		// ProcessTask пишет failure-статус сам. Если запись не удалась (Redis
		// недоступен), сообщение не подтверждаем — Reclaim вернёт его позже.
		if task, getErr := opts.Store.GetTask(ctx, delivery.TaskID); getErr != nil ||
			(task.Status != domain.TaskStatusSuccess && task.Status != domain.TaskStatusFailure) {
			return
		}
	}

	ack(ctx, opts, logger, delivery)
}

func ack(ctx context.Context, opts Options, logger *slog.Logger, delivery domain.TaskDelivery) {
	if err := opts.Queue.Ack(ctx, delivery.Receipt); err != nil {
		logger.Error("ack task", "err", err.Error())
	}
}
