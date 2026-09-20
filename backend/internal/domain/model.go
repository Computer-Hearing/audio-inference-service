package domain

import (
	"audio-inference-service/pkg"
	"net/http"
)

// Model - модель инференса
type Model struct {
	ID              int64  `json:"id,omitempty"`
	Title           string `json:"title"`
	ModelName       string `json:"model_name"`
	Description     string `json:"description"`
	SecondsPerChunk uint8  `json:"seconds_per_chunk"`
}

func (m Model) Validate() error {
	if m.Title == "" {
		return pkg.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "title is required",
		}
	}
	if m.ModelName == "" {
		return pkg.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "model_name is required",
		}
	}
	if m.SecondsPerChunk == 0 {
		return pkg.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "seconds_per_chunk is required",
		}
	}

	return nil
}
