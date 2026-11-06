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
	TargetOutputDatatype  = "TYPE_FP32"
	RawAudioInputName     = "RAW_AUDIO"
	RawAudioInputDatatype = "TYPE_UINT8"
	MaxTritonConcurrency  = 8
)

// ChunkOffsets возвращает сдвиги (слои) нарезки для заданной длины чанка.
// При чанке 1s слой со сдвигом на 1s ничего не даст при анализе - убираем
func ChunkOffsets(secondsPerChunk int) []int {
	if secondsPerChunk <= 1 {
		return []int{0}
	}
	return []int{0, 1}
}

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
