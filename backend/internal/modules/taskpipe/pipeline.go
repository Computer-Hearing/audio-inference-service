package taskpipe

import (
	"audio-inference-service/internal/domain"
	"audio-inference-service/internal/modules"
	"context"
	"log/slog"
	"time"
)

// StartPipeline - отвечает за запуск верокеров, которые задачи выполняют, а также диспетчера, который данные из бд берет
// связываются через канал.
func StartPipeline[P, R any](ctx context.Context, manager modules.TaskManager[P, R], predictor modules.FilePredictor[P]) {
	// Канал, через который Диспетчер передает задачи Воркерам
	jobsChan := make(chan domain.TaskPayload[P], 20)

	// Запускаем, например, 5 воркеров (горутин)
	for i := 0; i < 5; i++ {
		go func() {
			for job := range jobsChan {
				if err := predictor.ProcessTask(ctx, job); err != nil {
					// Логируем ошибку, задача либо останется в failure,
					// либо подхватится механизмом зависших задач (по таймауту)
					slog.Error("Failed to process task", "task_id", job.TaskID, "err", err.Error())
				}
			}
			slog.Debug("Channel closed", "worker_id", i+1)
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
				return
			case <-ticker.C:
				// Берем пачку до 10 задач за один раз
				jobs, err := manager.GetAndMarkProcessing(ctx, 10)
				if err != nil {
					slog.Error("Failed to get tasks", "err", err.Error())
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
