package pkg

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"mime"
	"net/http"
	"time"
)

type TaskStatus string

const (
	// User-defined
	UsernameCookieKey = "username"
	UsernameHeaderKey = "X-Username"
	ModelHeaderKey    = "X-Model"
	FormDataAudioKey  = "audio"
	DefaultUsername   = "Anonim"
	UsernameRX        = `^([a-zA-Z0-9_]+)-([a-zA-Z]+)-([0-9]+)$`

	// Validate Chunks
	SecondsPerAudioChunk = 2
	AudioWaveBucketsLen  = 40

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

func ParseJSONBody(r *http.Request, data any) error {
	defer r.Body.Close()

	if ct := r.Header.Get("Content-Type"); ct != "" {
		mediaType, _, err := mime.ParseMediaType(ct)
		if err != nil || mediaType != "application/json" {
			return APIError{
				StatusCode: http.StatusUnsupportedMediaType,
				Message:    "UNSUPPORTED_MEDIA_TYPE",
			}
		}
	}

	const maxBodyBytes = 1024 * 1024
	r.Body = http.MaxBytesReader(nil, r.Body, maxBodyBytes)

	dec := json.NewDecoder(r.Body)

	if err := dec.Decode(data); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return APIError{
				StatusCode: http.StatusRequestEntityTooLarge,
				Message:    "REQUEST_ENTITY_TOO_LARGE",
			}
		}
		return APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "INVALID_BODY",
		}
	}

	if err := dec.Decode(&struct{}{}); err != nil && err != io.EOF {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return APIError{
				StatusCode: http.StatusRequestEntityTooLarge,
				Message:    "REQUEST_ENTITY_TOO_LARGE",
			}
		}
		return APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "INVALID_BODY",
		}
	}
	return nil
}
