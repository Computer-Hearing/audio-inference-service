package dto

import (
	v1 "audio-inference-service/gen/inference/v1"
	"audio-inference-service/internal/domain"
)

func Task2DTO(task *domain.Task) *v1.Task {
	pbTask := &v1.Task{
		TaskId: task.TaskID.String(),
		Status: v1.TaskStatus(task.Status),
		Model:  task.Model,
		Result: nil,
	}

	if task.Result != nil {
		pbTask.Result = &v1.TaskResult{
			Filename: task.Result.Filename,
		}

		pbChunks := make([]*v1.Chunk, len(task.Result.Chunks))
		pbTask.Result.Chunks = pbChunks
		for i, chunk := range task.Result.Chunks {
			pbChunks[i] = &v1.Chunk{
				ChunkIndex:   int32(chunk.ChunkIndex),
				Layer:        int32(chunk.Layer),
				Offset:       int32(chunk.Offset),
				Category:     chunk.Category,
				Target:       chunk.Target,
				ErrorMessage: chunk.ErrorMessage,
			}
		}
	}

	return pbTask
}
