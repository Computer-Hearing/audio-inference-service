package repository

import (
	"audio-inference-service/pkg"
	"audio-inference-service/pkg/chunks"
	"context"
	"github.com/jackc/pgx/v4"
)

type TaskRepository interface {
	GetStatus(ctx context.Context, taskID pkg.Task) (pkg.TaskStatus, error)
	GetHistory(ctx context.Context, username pkg.Username) ([]*chunks.FileInferenceResult, error)
	DeleteHistory(ctx context.Context, username pkg.Username) error
	DeleteUser(ctx context.Context, username pkg.Username) error

	CreateTask(ctx context.Context, username pkg.Username, taskID pkg.Task, chunks chunks.AudioChunks, wave []float64) error
	StatusSuccess(ctx context.Context, tx pgx.Tx, taskID pkg.Task, result *chunks.FileInferenceResult) error
	StatusFailure(ctx context.Context, tx pgx.Tx, taskID pkg.Task) error
}
