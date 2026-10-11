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
)

// StartPipeline - отвечает за запуск верокеров, которые задачи выполняют, а также диспетчера, который данные из бд берет
// связываются через канал.
func StartPipeline(ctx context.Context, logger *slog.Logger, manager modules.TaskManager, predictor modules.FilePredictor) {
	// Канал, через который Диспетчер передает задачи Воркерам
	jobsChan := make(chan domain.TaskPayload, taskChanCapacity)

	// Запускаем воркеров (горутин)
	for i := range workersCount {
		go func() {
			for job := range jobsChan {
				logger.Debug("proccess task", "task_id", job.TaskID)

				if err := predictor.ProcessTask(ctx, job); err != nil {
					// Логируем ошибку, задача либо останется в failure,
					// либо подхватится механизмом зависших задач (по таймауту)
					logger.Error("failed to process task", "task_id", job.TaskID, "err", err.Error())
				}

			}
			logger.Debug("channel closed", "worker_id", i+1)
		}()
	}

	// Запускаем Диспетчера
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		defer close(jobsChan) // Закроет канал и остановит воркеры при отмене контекста

		for {
			select {
			case <-ctx.Done():
				logger.Debug("scheduler: context done")
				return
			case <-ticker.C:
				// Берем пачку до 10 задач за один раз
				jobs, err := manager.GetAndMarkProcessing(ctx, taskCountLimit)
				if err != nil {
					logger.Error("get task error", "err", err.Error())
					continue
				}

				// Раздаем задачи воркерам
				for _, job := range jobs {
					jobsChan <- job
				}
			}
		}
	}()
}
