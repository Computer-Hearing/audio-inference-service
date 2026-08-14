package chunks

import (
	"audio-inference-service/pkg"
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"os/exec"
)

func AudioWaveFromRequest(r *http.Request) ([]float64, error) {
	file, _, err := r.FormFile(pkg.FormDataAudioKey)
	if err != nil {
		return nil, &pkg.APIError{Message: err.Error(), StatusCode: http.StatusBadRequest}
	}
	defer file.Close()

	return audioWaveform(file, pkg.AudioWaveBucketsLen)
}

// AudioWaveform декодирует аудиофайл из multipart.File и возвращает массив
// амплитуд (0..1), разбитый на barCount "столбиков".
func audioWaveform(file multipart.File, barCount int) ([]float64, error) {
	// Декодируем аудио в сырой PCM через ffmpeg (моно, 16kHz, 16-bit)
	cmd := exec.Command("ffmpeg",
		"-i", "pipe:0",
		"-f", "s16le",
		"-acodec", "pcm_s16le",
		"-ar", "16000",
		"-ac", "1",
		"pipe:1",
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, &pkg.APIError{Message: err.Error(), StatusCode: http.StatusInternalServerError}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, &pkg.APIError{Message: err.Error(), StatusCode: http.StatusInternalServerError}
	}

	go func() {
		defer stdin.Close()
		io.Copy(stdin, file)
	}()

	if err := cmd.Wait(); err != nil {
		details := make(map[string]string)
		details["stderr"] = stderr.String()
		return nil, &pkg.APIError{Message: err.Error(), StatusCode: http.StatusInternalServerError, Details: details}
	}

	raw := stdout.Bytes()
	if len(raw) < 2 {
		return nil, &pkg.APIError{Message: "ffmpeg stdout empty", StatusCode: http.StatusBadRequest}
	}

	samples := make([]int16, len(raw)/2)
	for i := 0; i < len(samples); i++ {
		samples[i] = int16(binary.LittleEndian.Uint16(raw[i*2 : i*2+2]))
	}

	// Делим сэмплы на barCount групп и считаем RMS-амплитуду в каждой
	if barCount <= 0 {
		barCount = 40
	}

	bars := make([]float64, barCount)
	groupSize := len(samples) / barCount
	if groupSize == 0 {
		groupSize = 1
	}

	for i := 0; i < barCount; i++ {
		start := i * groupSize
		end := start + groupSize
		if end > len(samples) {
			end = len(samples)
		}
		if start >= end {
			bars[i] = 0
			continue
		}

		var sumSquares float64
		for _, s := range samples[start:end] {
			v := float64(s) / float64(math.MaxInt16)
			sumSquares += v * v
		}
		bars[i] = math.Sqrt(sumSquares / float64(end-start))
	}

	return bars, nil
}
