package pkg

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

func SendJSON(logger *slog.Logger, w http.ResponseWriter, data interface{}, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		logger.Warn("failed to encode json", "error", err.Error())
	}
}

func SendError(logger *slog.Logger, w http.ResponseWriter, err error, statusCode int) {
	SendJSON(logger, w, jsonError{Error: err.Error()}, statusCode)
}

type jsonError struct {
	Error string `json:"error"`
}
