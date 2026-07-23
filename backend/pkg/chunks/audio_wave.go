package chunks

import (
	"audio-inference-service/pkg/constants"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"os/exec"
)

func AudioWaveFromRequest(r *http.Request) ([]float64, error) {
	file, _, err := r.FormFile(constants.FormDataAudioKey)
	if err != nil {
		return nil, fmt.Errorf("получение файла из формы: %w", err)
	}
	defer file.Close()

	return audioWaveform(file, constants.AudioWaveBucketsLen)
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
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("запуск ffmpeg: %w", err)
	}

	go func() {
		defer stdin.Close()
		io.Copy(stdin, file)
	}()

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("ffmpeg завершился с ошибкой: %w, stderr: %s", err, stderr.String())
	}

	raw := stdout.Bytes()
	if len(raw) < 2 {
		return nil, fmt.Errorf("пустой аудиопоток после декодирования")
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
