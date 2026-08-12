package domain

import (
	"audio-inference-service/internal/chunks"
	"audio-inference-service/pkg"
	"crypto/md5"
	"fmt"

	"github.com/google/uuid"
)

type Task string

func (t Task) String() string {
	return string(t)
}

// GenerateTaskID - генерирует айди задачи
func GenerateTaskID(userName string) Task {
	return Task(fmt.Sprintf("%x", md5.Sum([]byte(userName+uuid.NewString()))))
}

// TaskPayload задача с типизированными данными для обработки
type TaskPayload[P any] struct {
	TaskID  Task
	Payload P
}

// AudioTaskPayload данные аудио-задачи
type AudioTaskPayload struct {
	ModelName string
	Chunks    chunks.AudioChunks
	Wave      []float64
}

type TaskResponse struct {
	TaskID Task      `json:"task_id"`
	Waves  []float64 `json:"waves"`
	Model  string    `json:"model,omitempty"`
}

// TaskResult задача с её статусом и результатом инференса (если он уже сохранён)
type TaskResult[R any] struct {
	TaskID Task           `json:"task_id"`
	Status pkg.TaskStatus `json:"status"`
	Model  string         `json:"model,omitempty"`
	Result *R             `json:"result,omitempty"`
}
