package pkg

import (
	"crypto/rand"
	"fmt"
	"log/slog"
	"math/big"
	"time"
)

type TaskStatus string

const (
	// User-defined
	UsernameCookieKey = "username"
	ModelHeader       = "X-Model"
	FormDataAudioKey  = "audio"
	DefaultUsername   = "Anonim"
	UsernameRX        = `^([a-zA-Z0-9_]+)-([a-zA-Z0-9]+)-([0-9]+)$`

	// Validate Chunks
	DefaultSecondsPerAudioChunk = 2
	MinTailChunkSeconds         = 0.5
	AudioWaveBucketsLen         = 40

	// Triton
	CategoryOutputName    = "category_output"
	TargetOutputName      = "target_output"
	DefaultModelName      = "cnn_predict_pipline"
	RawAudioInputName     = "RAW_AUDIO"
	RawAudioInputDatatype = "TYPE_UINT8"
	MaxTritonConcurrency  = 8

	UsernameFirstMin     = 4
	UsernameFirstMax     = 128
	UsernameSecond       = 13
	UsernameThird        = 10
	UsernameDelimiterLen = 1
)

// ChunkOffsetsSeconds сдвиги нарезки аудио в секундах для слоёв сегментации
var ChunkOffsetsSeconds = []int{0, 1}

func GetLoglevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func UsernameGenerator(first string) string {
	if first == "" {
		first = DefaultUsername
	}
	second := randWord()
	third := time.Now().Unix()

	return fmt.Sprintf("%s-%s-%d", first, second, third)
}

func randWord() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	result := make([]byte, UsernameSecond)
	for i := range result {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "thebestplayer"
		}
		result[i] = charset[n.Int64()]
	}
	return string(result)
}
