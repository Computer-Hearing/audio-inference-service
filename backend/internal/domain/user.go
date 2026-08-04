package domain

import (
	"audio-inference-service/pkg"
	"net/http"
)

const (
	MinUsernameLength = 4
	MaxUsernameLength = 128
)

type Username string

func (u Username) String() string {
	return string(u)
}

func (u Username) IsValid() error {
	if len(u) < MinUsernameLength || len(u) > MaxUsernameLength {
		return &pkg.APIError{Message: "Username length must be in 4-128", StatusCode: http.StatusBadRequest}
	}
	return nil
}

func ToUsername(username string) Username {
	return Username(username)
}
