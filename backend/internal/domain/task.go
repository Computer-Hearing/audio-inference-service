package domain

import (
	"audio-inference-service/internal/modules/audio"
	"crypto/md5"
	"fmt"

	"github.com/google/uuid"
)

// Task - задача. Это структура начала и окончания инференса.
// В начале есть только TaskID, Model и Status (который равен 'pending').
// В конце Status меняется на success/failure, также если success то Result еще отдается
type Task struct {
	TaskID TaskID               `json:"task_id"`
	Model  string               `json:"model"`
	Status TaskStatus           `json:"status"`
	Result *FileInferenceResult `json:"result,omitempty"`
}

type TaskID string

func (t TaskID) String() string {
	return string(t)
}

// GenerateTaskID - генерирует айди задачи
func GenerateTaskID(userName string) TaskID {
	return TaskID(fmt.Sprintf("%x", md5.Sum([]byte(userName+uuid.NewString()))))
}

// TaskStatus статус задачи
type TaskStatus int32

const (
	TaskStatusUnspecified TaskStatus = 0
	TaskStatusSuccess     TaskStatus = 1
	TaskStatusFailure     TaskStatus = 2
	TaskStatusPending     TaskStatus = 3
	TaskStatusProcessing  TaskStatus = 4
)

// TaskPayload -
type TaskPayload struct {
	TaskID  TaskID
	Payload AudioTaskPayload
}

// AudioTaskPayload данные аудио-задачи
type AudioTaskPayload struct {
	ModelName string
	Chunks    audio.AudioChunks
}
