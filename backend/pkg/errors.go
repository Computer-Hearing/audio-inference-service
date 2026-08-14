package pkg

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
)

const (
	AuthorErrorSqlite   string = "sqlite"
	AuthorErrorTriton   string = "triton"
	AuthorErrorPipeline string = "pipeline"
)

type APIError struct {
	StatusCode int               `json:"status_code"`
	Message    string            `json:"message"`
	Details    map[string]string `json:"details"`
}

func (e APIError) Error() string {
	details := e.stringDetails()
	if details == "" {
		return e.Message
	}
	return fmt.Sprintf("%s. %s", e.Message, details)
}

func (e APIError) stringDetails() string {
	if len(e.Details) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("Details: ")
	i := 0
	for k, v := range e.Details {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(v)
		i++
	}
	return b.String()
}

func HandleError(w http.ResponseWriter, logger *slog.Logger, err error) {
	var errApi APIError
	if errors.As(err, &errApi) {
		SendError(logger, w, &errApi, errApi.StatusCode)
		return
	}
	SendError(logger, w, err, http.StatusInternalServerError)
}
