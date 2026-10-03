package dto

import "audio-inference-service/internal/domain"

func TaskStatusToString(t domain.TaskStatus) string {
	switch t {
	case domain.TaskStatusSuccess:
		return "success"
	case domain.TaskStatusFailure:
		return "failure"
	case domain.TaskStatusPending:
		return "pending"
	case domain.TaskStatusProcessing:
		return "processing"
	default:
		return "unspecified"
	}
}

func TaskStatusFromString(status string) domain.TaskStatus {
	switch status {
	case "success":
		return domain.TaskStatusSuccess
	case "failure":
		return domain.TaskStatusFailure
	case "pending":
		return domain.TaskStatusPending
	case "processing":
		return domain.TaskStatusProcessing
	default:
		return domain.TaskStatusUnspecified
	}
}
