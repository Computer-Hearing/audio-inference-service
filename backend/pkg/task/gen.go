package task

import (
	"crypto/md5"
	"fmt"

	"github.com/google/uuid"
)

// GenerateTaskID - генерирует айди задачи
func GenerateTaskID(userName string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(userName+uuid.NewString())))
}
