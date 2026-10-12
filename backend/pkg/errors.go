package pkg

import (
	"errors"
	"fmt"

	"connectrpc.com/connect"
)

// APIError - ошибка приложения с HTTP-статусом и структурированными деталями.
// StatusCode используется legacy REST-слоем, ConnectCode - для RPC.
type APIError struct {
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
	Details    any    `json:"details,omitempty"`

	// Cause - обёрнутая ошибка, участвует в errors.Is/errors.As.
	Cause error `json:"-"`
}

func (e APIError) Error() string {
	if e.Details == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Details)
}

// Unwrap возвращает обёрнутую ошибку, чтобы работали errors.Is и errors.As.
func (e APIError) Unwrap() error {
	return e.Cause
}

// ConnectCode возвращает gRPC/connect-код ошибки.
func (e APIError) ConnectCode() connect.Code {
	switch e.StatusCode {
	case 400:
		return connect.CodeInvalidArgument
	case 401:
		return connect.CodeUnauthenticated
	case 403:
		return connect.CodePermissionDenied
	case 404:
		return connect.CodeNotFound
	case 409:
		return connect.CodeAlreadyExists
	case 413:
		return connect.CodeResourceExhausted
	case 429:
		return connect.CodeResourceExhausted
	case 503:
		return connect.CodeUnavailable
	case 504:
		return connect.CodeDeadlineExceeded
	default:
		return connect.CodeInternal
	}
}

// WithCause оборачивает исходную ошибку.
func (e APIError) WithCause(err error) APIError {
	e.Cause = err
	return e
}

// WithDetails добавляет структурированные детали.
func (e APIError) WithDetails(details any) APIError {
	e.Details = details
	return e
}

// ConnectError оборачивает APIError в connect-ошибку с корректным кодом.
func ConnectError(err error) error {
	var apiErr APIError
	if AsAPIError(err, &apiErr) {
		return connect.NewError(apiErr.ConnectCode(), err)
	}
	return connect.NewError(connect.CodeUnknown, err)
}

// AsAPIError ищет APIError в цепочке ошибок, включая обёрнутые через %w.
func AsAPIError(err error, target *APIError) bool {
	for err != nil {
		if apiErr, ok := errors.AsType[APIError](err); ok {
			*target = apiErr
			return true
		}
		unwrapper, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = unwrapper.Unwrap()
	}
	return false
}

// -------------------- конструкторы --------------------

func NewBadRequestError(message string) APIError {
	return APIError{StatusCode: 400, Message: message}
}

func NewBadRequestWithDetails(message string, details any) APIError {
	return APIError{StatusCode: 400, Message: message, Details: details}
}

func NewUnauthorizedError(message string) APIError {
	return APIError{StatusCode: 401, Message: message}
}

func NewNotFoundError(message string) APIError {
	return APIError{StatusCode: 404, Message: message}
}

func NewNotFoundWithDetails(message string, details any) APIError {
	return APIError{StatusCode: 404, Message: message, Details: details}
}

func NewConflictError(message string) APIError {
	return APIError{StatusCode: 409, Message: message}
}

func NewPayloadTooLargeError(message string) APIError {
	return APIError{StatusCode: 413, Message: message}
}

func NewInternalError(message string) APIError {
	return APIError{StatusCode: 500, Message: message}
}

func NewInternalWithDetails(message string, details any) APIError {
	return APIError{StatusCode: 500, Message: message, Details: details}
}

func NewServiceUnavailableError(message string) APIError {
	return APIError{StatusCode: 503, Message: message}
}

func NewServiceUnavailableWithDetails(message string, details any) APIError {
	return APIError{StatusCode: 503, Message: message, Details: details}
}
