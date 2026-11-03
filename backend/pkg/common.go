package pkg

import (
	"log/slog"
)

type TaskStatus string

const (
	// User-defined
	ModelHeader      = "X-Model"
	FormDataAudioKey = "audio"

	// Validate Chunks
	DefaultSecondsPerAudioChunk = 2
	MinTailChunkSeconds         = 0.5
	AudioWaveBucketsLen         = 40

	// Triton
	CategoryOutputName    = "category_output"
	TargetOutputName      = "target_output"
	DefaultModelName      = "cnn_predict_pipeline_2"
	RawAudioInputName     = "RAW_AUDIO"
	RawAudioInputDatatype = "TYPE_UINT8"
	MaxTritonConcurrency  = 8
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
