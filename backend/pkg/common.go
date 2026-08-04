package pkg

import "log/slog"

type TaskStatus string

const (
	// User
	UsernameCookieKey = "username"
	UsernameHeaderKey = "X-Username"
	FormDataAudioKey  = "audio"

	// Validate Chunks
	SecondsPerAudioChunk = 2
	AudioWaveBucketsLen  = 40

	// Triton
	CategoryOutputName   = "category_output"
	TargetOutputName     = "target_output"
	PipelineModelName    = "cnn_predict_pipline"
	RawAudioInputName    = "RAW_AUDIO"
	MaxTritonConcurrency = 4

	// Статусы задач
	StatusProcessing TaskStatus = "processing"
	StatusSuccess    TaskStatus = "success"
	StatusError      TaskStatus = "error"
)

func Loglevel(level string) slog.Level {
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
		return slog.LevelDebug
	}
}
