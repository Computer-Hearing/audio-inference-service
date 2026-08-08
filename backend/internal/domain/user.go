package domain

import (
	"audio-inference-service/pkg"
	"net/http"
	"regexp"
)

type Username string

func (u Username) String() string {
	return string(u)
}

func (u Username) IsValid() error {
	if len(u) < pkg.UsernameFirstMin+pkg.UsernameSecond+pkg.UsernameThird+pkg.UsernameDelimiterLen*2 {
		return &pkg.APIError{Message: "invalid username format", StatusCode: http.StatusBadRequest}
	}
	if len(u) > pkg.UsernameFirstMax+pkg.UsernameSecond+pkg.UsernameThird+pkg.UsernameDelimiterLen*2 {
		return &pkg.APIError{Message: "invalid username format", StatusCode: http.StatusBadRequest}
	}

	rx, err := regexp.Compile(pkg.UsernameRX)
	if err != nil {
		return &pkg.APIError{Message: err.Error(), StatusCode: http.StatusBadRequest}
	}
	if result := rx.MatchString(u.String()); !result {
		return &pkg.APIError{Message: "incorrect username format", StatusCode: http.StatusBadRequest}
	}

	return nil
}

func ToUsername(username string) Username {
	return Username(username)
}
