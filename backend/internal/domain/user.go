package domain

import (
	"audio-inference-service/pkg"
	"regexp"
)

type Username string

func (u Username) String() string {
	return string(u)
}

func (u Username) IsValid() error {
	if len(u) < pkg.UsernameFirstMin+pkg.UsernameSecond+pkg.UsernameThird+pkg.UsernameDelimiterLen*2 {
		return pkg.NewBadRequestError("invalid username format")
	}
	if len(u) > pkg.UsernameFirstMax+pkg.UsernameSecond+pkg.UsernameThird+pkg.UsernameDelimiterLen*2 {
		return pkg.NewBadRequestError("invalid username format")
	}

	rx, err := regexp.Compile(pkg.UsernameRX)
	if err != nil {
		return pkg.NewBadRequestError(err.Error())
	}
	if result := rx.MatchString(u.String()); !result {
		return pkg.NewBadRequestError("incorrect username format")
	}

	return nil
}
