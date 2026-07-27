package service

import (
	"audio-inference-service/pkg"
	"audio-inference-service/pkg/chunks"
	"context"
	"github.com/jackc/pgx/v4"
)

type FilePredictor interface {
	ProcessTask(ctx context.Context) error
}

type TaskLoader interface {
	CreateTask(
		ctx context.Context, username pkg.Username, taskID pkg.Task, chunks *chunks.AudioChunks, waves []float64) error
	GetStatus(ctx context.Context, taskID string, chunks *chunks.AudioChunks) (pkg.TaskStatus, error)
	GetOneWithBlock(ctx context.Context) (pgx.Tx, pkg.Task, chunks.AudioChunks, error)
	GetHistory(ctx context.Context, taskID string, chunks *chunks.AudioChunks) ([]chunks.FileInferenceResult, error)
}
