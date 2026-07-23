package service

import (
	"audio-inference-service/pkg/chunks"

	"github.com/google/uuid"
)

type FilePredictor interface {
	Predict(taskID uuid.UUID, chunks *chunks.AudioChunks) ([]string, []string, error)
}

type TaskLoader interface {
	Load(taskID uuid.UUID, chunks *chunks.AudioChunks) ([]string, []string, error)
	LoadAll(taskID uuid.UUID, chunks *chunks.AudioChunks) ([]string, []string, error)
	Delete(taskID uuid.UUID, chunks *chunks.AudioChunks) error
	DeleteAll(taskID uuid.UUID, chunks *chunks.AudioChunks) error
}
