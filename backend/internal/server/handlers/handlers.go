package handlers

import (
	"audio-inference-service/pkg/chunks"
	"audio-inference-service/pkg/task"
	"net/http"
)

type Handlers struct {
}

func (h *Handlers) FilePipeline(w http.ResponseWriter, r *http.Request) {
	userName := "vladosik" //TODO нужно мидлваре считывающая куки пользователя

	ch, err := chunks.ChunksFromRequest(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	taskID := task.GenerateTaskID(userName)
	// TODO тут между этими запустить горутину с выполнением задачи

	waves, err := chunks.AudioWaveFromRequest(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// TODO: доделывать
	_, _, _ = taskID, ch, waves
}
