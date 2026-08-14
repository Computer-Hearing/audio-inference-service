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
	"strconv"
	"strings"
)

type AudioLayer struct {
	Offset int      `json:"offset"` // сдвиг в секундах
	Chunks [][]byte `json:"chunks"` // куски по 2 секунды
}

type AudioChunks struct {
	Filename string       `json:"filename"` // исходное имя файла
	Layers   []AudioLayer `json:"layers"`   // слои сегментации
}

func ChunksFromRequest(r *http.Request) (*AudioChunks, error) {
	file, header, err := r.FormFile(pkg.FormDataAudioKey)
	if err != nil {
		return nil, &pkg.APIError{Message: err.Error(), StatusCode: http.StatusBadRequest}
	}
	defer file.Close()

	return splitAudio(file, header, pkg.SecondsPerAudioChunk) // 2 секунды на кусок
}

// Берем мультпарт-файл, дробим его на слои с разными сдвигами и выдаем массивчики чанков звука
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

	// Длительность файла нужна, чтобы в слоях со сдвигом отбросить неполный хвостовой чанк
	duration, err := audioDurationSeconds(inputPath)
	if err != nil {
		return nil, err
	}

	layers := make([]AudioLayer, 0, len(pkg.ChunkOffsetsSeconds))
	for _, offset := range pkg.ChunkOffsetsSeconds {
		// Для слоя со сдвигом оставляем только полные куски: полное количество ровных чанков от сдвига до конца
		trim := 0.0
		if offset > 0 {
			fullChunks := int(duration-float64(offset)) / chunkSeconds
			if fullChunks <= 0 {
				continue // файл короче сдвига — слой пустой
			}
			trim = float64(fullChunks * chunkSeconds)
		}

		layerDir := filepath.Join(tmpDir, fmt.Sprintf("layer_%d", offset))
		matches, err := segmentLayer(inputPath, layerDir, ext, offset, chunkSeconds, trim)
		if err != nil {
			return nil, err
		}

		// перебираем чанки, открываем и добавляем в слайсик
		chunks := make([][]byte, 0, len(matches))
		for _, m := range matches {
			b, err := os.ReadFile(m)
			if err != nil { // если была ошибка, то не расстраиваемся, просто добавляем пустой кусочек и скипаем
				// перед вызовом trion проверим, что не пустой, если да, то это ошибка и на фронт ошибку в чанке отдадим
				chunks = append(chunks, []byte{})
				continue
			}
			chunks = append(chunks, b)
		}
		if len(chunks) > 0 {
			layers = append(layers, AudioLayer{Offset: offset, Chunks: chunks})
		}
	}

	return &AudioChunks{
		Filename: header.Filename,
		Layers:   layers,
	}, nil
}

// segmentLayer режет inputPath на чанки по chunkSeconds секунд, начиная со сдвига offset секунд
func segmentLayer(inputPath, layerDir, ext string, offset, chunkSeconds int, trim float64) ([]string, error) {
	if err := os.MkdirAll(layerDir, 0o755); err != nil {
		return nil, &pkg.APIError{Message: err.Error(), StatusCode: http.StatusInternalServerError}
	}

	// Паттерн для чанков вида: chunk_001, chunk_002, chunk_003...
	outPattern := filepath.Join(layerDir, "chunk_%03d"+ext)

	args := []string{}
	if offset > 0 {
		args = append(args, "-ss", strconv.Itoa(offset))
	}
	args = append(args, "-i", inputPath)
	if trim > 0 {
		args = append(args, "-to", strconv.FormatFloat(trim, 'f', -1, 64))
	}
	args = append(args,
		"-f", "segment",
		"-segment_time", strconv.Itoa(chunkSeconds),
		"-c", "copy",
		"-y",
		outPattern,
	)

	// Берем тулзу cmd
	cmd := exec.Command("ffmpeg", args...)

	// Буфер ошибок передаем, это ж из cmd, а не го вызов
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	// выполняем
	if err := cmd.Run(); err != nil {
		return nil, &pkg.APIError{
			Message:    fmt.Sprintf("ffmpeg: %s", err.Error()),
			StatusCode: http.StatusInternalServerError,
			Details:    map[string]string{"stderr": stderr.String()},
		}
	}

	// Получаем список названий файлов-чанков из временной этой папки
	matches, err := filepath.Glob(filepath.Join(layerDir, "chunk_*"+ext))
	if err != nil {
		return nil, &pkg.APIError{Message: err.Error(), StatusCode: http.StatusInternalServerError}
	}
	sort.Strings(matches) // chunk_000, chunk_001, ... — сортируем на всякий случай

	return matches, nil
}

// audioDurationSeconds возвращает длительность аудиофайла в секундах
func audioDurationSeconds(inputPath string) (float64, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		inputPath,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return 0, &pkg.APIError{
			Message:    fmt.Sprintf("ffprobe: %s", err.Error()),
			StatusCode: http.StatusInternalServerError,
			Details:    map[string]string{"stderr": stderr.String()},
		}
	}

	duration, err := strconv.ParseFloat(strings.TrimSpace(stdout.String()), 64)
	if err != nil {
		return 0, &pkg.APIError{Message: fmt.Sprintf("ffprobe duration parse: %s", err.Error()), StatusCode: http.StatusInternalServerError}
	}

	return duration, nil
}

// InferenceResult результат по одному чанку
type InferenceResult struct {
	CategoryLogits []float32 `json:"category_logits"` // 5 значений
	TargetLogits   []float32 `json:"target_logits"`   // 7 значений
}

// ChunkResult результат инференса одного чанка с привязкой к его слою и индексу
type ChunkResult struct {
	ChunkIndex   int       `json:"chunk_index"`      // индекс внутри слоя
	Layer        int       `json:"layer"`            // номер слоя
	Offset       int       `json:"offset,omitempty"` // сдвиг слоя в секундах
	Category     []float32 `json:"category"`
	Target       []float32 `json:"target"`
	Err          error     `json:"-"`
	ErrorMessage string    `json:"error,omitempty"`
}

// FileInferenceResult агрегированный результат по всему файлу
type FileInferenceResult struct {
	Filename string        `json:"filename"`
	Chunks   []ChunkResult `json:"chunks"` // в порядке (слой, индекс)
}
