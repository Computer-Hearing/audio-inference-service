package pkg

import (
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

type TaskResponse struct {
	TaskID Task      `json:"task_id"`
	Waves  []float64 `json:"waves"`
}
