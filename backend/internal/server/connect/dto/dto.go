package dto

import (
	v1 "audio-inference-service/gen/inference/v1"
	"audio-inference-service/internal/domain"
	"audio-inference-service/internal/modules/models"
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

func Models2DTO(models []models.ModelConfig) []*v1.Model {
	dtoModels := make([]*v1.Model, len(models))
	for i, model := range models {
		dtoModels[i] = Model2DTO(model)
	}
	return dtoModels
}

func Model2DTO(model models.ModelConfig) *v1.Model {
	return &v1.Model{
		Name:               model.Name,
		ProjectTitle:       model.ProjectTitle,
		ProjectDescription: model.ProjectDescription,
		SecondsPerChunk:    int32(model.SecondsPerChunk),
		TargetClassesNum:   int32(model.TargetClassesNum),
		CategoryClassesNum: int32(model.CategoryClassesNum),
		Ready:              model.Ready,
		State:              model.State,
	}
}
