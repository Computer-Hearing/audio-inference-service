package chunks

import (
	"audio-inference-service/pkg"
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

type AudioChunks struct {
	Filename string   // исходное имя файла
	Chunks   [][]byte // куски по 2 секунды
}

func ChunksFromRequest(r *http.Request) (*AudioChunks, error) {
	file, header, err := r.FormFile(pkg.FormDataAudioKey)
	if err != nil {
		return nil, &pkg.APIError{Message: err.Error(), StatusCode: http.StatusBadRequest}
	}
	defer file.Close()

	return splitAudio(file, header, pkg.SecondsPerAudioChunk) // 2 секунды на кусок
}

// Берем мультипарт-файл дробим его и выдаем массивчик чанков звука
func splitAudio(file multipart.File, header *multipart.FileHeader, chunkSeconds int) (*AudioChunks, error) {
	// Создаем временную папку с суффиксом, для уникальности
	tmpDir, err := os.MkdirTemp("", "audio_split")
	if err != nil {
		return nil, &pkg.APIError{Message: err.Error(), StatusCode: http.StatusInternalServerError}
	}
	defer os.RemoveAll(tmpDir)

	// Расширение файла, если есть
	ext := filepath.Ext(header.Filename)
	if ext == "" {
		ext = ".wav" // на случай, если расширения нет
	}

	// Создаем временный файл (ffmpeg - утилита, берет файла по пути, то есть файл сохраненный)
	inputPath := filepath.Join(tmpDir, "input"+ext)
	dst, err := os.Create(inputPath)
	if err != nil {
		return nil, &pkg.APIError{Message: err.Error(), StatusCode: http.StatusInternalServerError}
	}
	// Копируем его туда данные мультпарт
	if _, err := io.Copy(dst, file); err != nil {
		dst.Close()
		return nil, &pkg.APIError{Message: err.Error(), StatusCode: http.StatusInternalServerError}
	}
	dst.Close()

	// Паттерн для чанков вида: chunk_001, chunk_002, chunk_003...
	outPattern := filepath.Join(tmpDir, "chunk_%03d"+ext)

	// Берем тулзу cmd
	cmd := exec.Command("ffmpeg",
		"-i", inputPath,
		"-f", "segment",
		"-segment_time", fmt.Sprintf("%d", chunkSeconds),
		"-c", "copy",
		"-y",
		outPattern,
	)

	// Буфер ошибок передаем, это ж из cmd, а не го вызов
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	// выполняем
	if err := cmd.Run(); err != nil {
		return nil, &pkg.APIError{Message: err.Error(), StatusCode: http.StatusInternalServerError}
	}

	// Получаем список названий файлов-чанков из временной этой папки
	matches, err := filepath.Glob(filepath.Join(tmpDir, "chunk_*"+ext))
	if err != nil {
		return nil, &pkg.APIError{Message: err.Error(), StatusCode: http.StatusInternalServerError}
	}
	sort.Strings(matches) // chunk_000, chunk_001, ... — сортируем на всякий случай

	// создаем результирующий слайсик
	chunks := make([][]byte, 0, len(matches))
	// перебираем чанки, открываем и добавляем в слайсик
	for _, m := range matches {
		b, err := os.ReadFile(m)
		if err != nil { // если была ошибка, то не расстраиваемся, просто добавляем пустой кусочек и скипаем
			// перед вызовом trion проверим, что не пустой, если да, то это ошибка и на фронт ошибку в чанке отдадим
			chunks = append(chunks, []byte{})
			continue
		}
		chunks = append(chunks, b)
	}

	return &AudioChunks{
		Filename: header.Filename,
		Chunks:   chunks,
	}, nil
}

// InferenceResult результат по одному чанку
type InferenceResult struct {
	CategoryLogits []float32 // 5 значений
	TargetLogits   []float32 // 7 значений
}

// ChunkResult результат инференса одного чанка с привязкой к его индексу
type ChunkResult struct {
	ChunkIndex int
	Category   []float32
	Target     []float32
	Err        error
}

// FileInferenceResult агрегированный результат по всему файлу
type FileInferenceResult struct {
	Filename string
	Chunks   []ChunkResult // в исходном порядке чанков
}
