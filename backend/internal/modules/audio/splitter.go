package audio

import (
	"audio-inference-service/pkg"
	"bytes"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// AudioLayer слой сегментации аудио
type AudioLayer struct {
	Offset int      `json:"offset"`
	Chunks [][]byte `json:"chunks"`
}

// AudioChunks нарезанное аудио по слоям
type AudioChunks struct {
	Filename string       `json:"filename"`
	Layers   []AudioLayer `json:"layers"`
}

type Options struct {
	Data            []byte
	Filename        string
	SecondsPerChunk int
}

// Split нарезает аудиофайл из proto-запроса на слои чанков длительностью chunkSeconds.
// filename нужен только для определения расширения (ffmpeg определяет кодек по содержимому)
func Split(opts Options) (*AudioChunks, error) {
	data, filename, secondsPerChunk := opts.Data, opts.Filename, opts.SecondsPerChunk

	if len(data) == 0 {
		return nil, &pkg.APIError{Message: "audio file is empty", StatusCode: http.StatusBadRequest}
	}
	if secondsPerChunk <= 0 {
		return nil, &pkg.APIError{Message: "seconds_per_chunk is empty", StatusCode: http.StatusBadRequest}
	}

	// создаем временную директорию, куда чанки будут резаться
	tmpDir, err := os.MkdirTemp("", "audio_split")
	if err != nil {
		return nil, &pkg.APIError{Message: err.Error(), StatusCode: http.StatusInternalServerError}
	}
	defer os.RemoveAll(tmpDir)

	// получаем расширение файла, по умолчанию считаем как wav
	ext := filepath.Ext(filename)
	if ext == "" {
		ext = ".wav"
	}

	// сохраняем файл, который из proto пришел в эту временную директорию
	inputPath := filepath.Join(tmpDir, "input"+ext)
	if err := os.WriteFile(inputPath, data, 0o600); err != nil {
		return nil, &pkg.APIError{Message: err.Error(), StatusCode: http.StatusInternalServerError}
	}

	// получаем длительность звука
	duration, err := audioDurationSeconds(inputPath)
	if err != nil {
		return nil, err
	}

	// идем по слоям
	layers := make([]AudioLayer, 0, len(pkg.ChunkOffsetsSeconds))
	for _, offset := range pkg.ChunkOffsetsSeconds {
		// trim - сколько по времени этот слой будет
		// offset от какого места ffmpeg будет резать по звуку (0 - 0s, 1 - 1s, ...)
		trim := 0.0
		if offset > 0 {
			fullChunks := int(duration-float64(offset)) / secondsPerChunk
			if fullChunks <= 0 {
				continue
			}
			trim = float64(fullChunks * secondsPerChunk)
		}

		// режем конкретный слой
		layerDir := filepath.Join(tmpDir, fmt.Sprintf("layer_%d", offset))
		matches, err := segmentLayer(inputPath, layerDir, ext, offset, secondsPerChunk, trim)
		if err != nil {
			return nil, err
		}

		// обрезаем звостовой чанк, если его продолжительность меньше pkg.MinTailChunkSeconds (0,5s)
		if offset == 0 {
			fullChunks := int(duration) / secondsPerChunk
			tail := duration - float64(fullChunks*secondsPerChunk)
			if fullChunks > 0 && len(matches) > fullChunks && tail < pkg.MinTailChunkSeconds {
				matches = matches[:fullChunks]
			}
		}

		// пробегаемся по каждому названию чанка, открываем сам чанк и в массив добавляем
		chunks := make([][]byte, 0, len(matches))
		for _, m := range matches {
			b, err := os.ReadFile(m)
			if err != nil {
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
		Filename: filename,
		Layers:   layers,
	}, nil
}

// segmentLayer режет inputPath на чанки по chunkSeconds секунд, начиная со сдвига offset
func segmentLayer(inputPath, layerDir, ext string, offset, chunkSeconds int, trim float64) ([]string, error) {
	// создаем временную папку под слой
	if err := os.MkdirAll(layerDir, 0o755); err != nil {
		return nil, &pkg.APIError{Message: err.Error(), StatusCode: http.StatusInternalServerError}
	}

	// паттерн для нарезания файлов (chunk_001.wav, chunk_002.wav, ...)
	outPattern := filepath.Join(layerDir, "chunk_%03d"+ext)

	var args []string
	if offset > 0 {
		// флаг -ss - показывает начинать обработку с конкретной точки звука
		args = append(args, "-ss", strconv.Itoa(offset))
	}

	// флаг -i - какой файл подавать на вход
	args = append(args, "-i", inputPath)
	if trim > 0 {
		// флаг -to указывает до какого момента мы звук резать будем
		args = append(args, "-to", strconv.FormatFloat(trim, 'f', -1, 64))
	}
	args = append(args,
		"-f", "segment",
		"-segment_time", strconv.Itoa(chunkSeconds),
		"-c", "copy",
		"-y",
		outPattern,
	)

	cmd := exec.Command("ffmpeg", args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, &pkg.APIError{
			Message:    fmt.Sprintf("ffmpeg: %s", err.Error()),
			StatusCode: http.StatusInternalServerError,
			Details:    map[string]string{"stderr": stderr.String()},
		}
	}

	// получаем названия файлов всех нарезанных чанков
	matches, err := filepath.Glob(filepath.Join(layerDir, "chunk_*"+ext))
	if err != nil {
		return nil, &pkg.APIError{Message: err.Error(), StatusCode: http.StatusInternalServerError}
	}
	sort.Strings(matches)

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
