package audio

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

// ChunksFromRequest извлекает аудио из multipart-запроса и нарезает на чанки
func ChunksFromRequest(r *http.Request) (*AudioChunks, error) {
	file, header, err := r.FormFile(pkg.FormDataAudioKey)
	if err != nil {
		return nil, &pkg.APIError{Message: err.Error(), StatusCode: http.StatusBadRequest}
	}
	defer file.Close()

	return splitAudio(file, header, pkg.DefaultSecondsPerAudioChunk)
}

// splitAudio режет файл на слои с разными сдвигами
func splitAudio(file multipart.File, header *multipart.FileHeader, chunkSeconds int) (*AudioChunks, error) {
	tmpDir, err := os.MkdirTemp("", "audio_split")
	if err != nil {
		return nil, &pkg.APIError{Message: err.Error(), StatusCode: http.StatusInternalServerError}
	}
	defer os.RemoveAll(tmpDir)

	ext := filepath.Ext(header.Filename)
	if ext == "" {
		ext = ".wav"
	}

	inputPath := filepath.Join(tmpDir, "input"+ext)
	dst, err := os.Create(inputPath)
	if err != nil {
		return nil, &pkg.APIError{Message: err.Error(), StatusCode: http.StatusInternalServerError}
	}
	if _, err := io.Copy(dst, file); err != nil {
		dst.Close()
		return nil, &pkg.APIError{Message: err.Error(), StatusCode: http.StatusInternalServerError}
	}
	dst.Close()

	duration, err := audioDurationSeconds(inputPath)
	if err != nil {
		return nil, err
	}

	layers := make([]AudioLayer, 0, len(pkg.ChunkOffsetsSeconds))
	for _, offset := range pkg.ChunkOffsetsSeconds {
		trim := 0.0
		if offset > 0 {
			fullChunks := int(duration-float64(offset)) / chunkSeconds
			if fullChunks <= 0 {
				continue
			}
			trim = float64(fullChunks * chunkSeconds)
		}

		layerDir := filepath.Join(tmpDir, fmt.Sprintf("layer_%d", offset))
		matches, err := segmentLayer(inputPath, layerDir, ext, offset, chunkSeconds, trim)
		if err != nil {
			return nil, err
		}

		if offset == 0 {
			fullChunks := int(duration) / chunkSeconds
			tail := duration - float64(fullChunks*chunkSeconds)
			if fullChunks > 0 && len(matches) > fullChunks && tail < pkg.MinTailChunkSeconds {
				matches = matches[:fullChunks]
			}
		}

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
		Filename: header.Filename,
		Layers:   layers,
	}, nil
}

// segmentLayer режет inputPath на чанки по chunkSeconds секунд, начиная со сдвига offset
func segmentLayer(inputPath, layerDir, ext string, offset, chunkSeconds int, trim float64) ([]string, error) {
	if err := os.MkdirAll(layerDir, 0o755); err != nil {
		return nil, &pkg.APIError{Message: err.Error(), StatusCode: http.StatusInternalServerError}
	}

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
