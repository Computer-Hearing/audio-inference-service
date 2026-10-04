// Command openfile запрашивает путь до аудиофайла, читает его,
// сериализует в CreateTaskRequest и печатает готовый JSON для buf curl (для postman тоже подойдет)
package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// maxFileBytes ограничивает размер исходного файла, чтобы base64 поместился
// в лимит ReadMaxBytes (~4 МБ) с запасом на base64-раздутие.
const maxFileBytes = 3 << 20 // 3 МБ

// CreateTaskRequest повторяет контракт inference.v1.api.CreateTaskRequest.
type CreateTaskRequest struct {
	AudioFile []byte `json:"audio_file,omitempty"`
	Filename  string `json:"filename,omitempty"`
	ModelName string `json:"model_name,omitempty"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Путь до аудиофайла: ")
	path, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read path: %w", err)
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("path is empty")
	}

	fmt.Print("Название модели (например cnn_predict_pipeline_2): ")
	model, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read model: %w", err)
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return fmt.Errorf("model name is empty")
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", path)
	}
	if info.Size() > maxFileBytes {
		return fmt.Errorf("file is %d bytes, limit is %d bytes", info.Size(), int64(maxFileBytes))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	req := CreateTaskRequest{
		AudioFile: data,
		Filename:  filepath.Base(path),
		ModelName: model,
	}

	// encoding/json кодирует []byte как base64 — ровно то, что ждёт protobuf-JSON.
	encoded, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	if err := os.WriteFile("request.json", encoded, 0o600); err != nil {
		return fmt.Errorf("write request.json: %w", err)
	}

	fmt.Fprintf(os.Stderr, "OK: %s (%d bytes) -> request.json (%d bytes, base64 %d)\n",
		req.Filename, len(data), len(encoded), base64.StdEncoding.EncodedLen(len(data)))

	return nil
}
