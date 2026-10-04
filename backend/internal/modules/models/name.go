package models

import (
	"regexp"
	"strconv"
)

// modelNameRe паттерн имени модели: <modelname>_<seconds_per_chunk>
var modelNameRe = regexp.MustCompile(`^(.+)_(\d+)$`)

// ParseModelName парсит имя модели вида <modelname>_<seconds_per_chunk>.
// Возвращает имя модели, длительность чанка в секундах и true если имя валидно.
func ParseModelName(name string) (modelName string, secondsPerChunk int, ok bool) {
	matches := modelNameRe.FindStringSubmatch(name)
	if matches == nil {
		return "", 0, false
	}

	seconds, err := strconv.Atoi(matches[2])
	if err != nil || seconds <= 0 {
		return "", 0, false
	}

	return matches[1], seconds, true
}
