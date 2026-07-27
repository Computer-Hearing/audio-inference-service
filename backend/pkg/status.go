package pkg

type TaskStatus string

const (
	StatusProcessing TaskStatus = "processing"
	StatusSuccess    TaskStatus = "success"
	StatusError      TaskStatus = "error"
)
