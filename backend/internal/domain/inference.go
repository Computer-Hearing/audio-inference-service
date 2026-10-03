package domain

// InferenceResult результат по одному чанку от самой triton-модели
type InferenceResult struct {
	CategoryLogits []float32 `json:"category_logits"`
	TargetLogits   []float32 `json:"target_logits"`
}

// ChunkResult результат инференса одного чанка с привязкой к его слою и индексу. Это после обработки InferenceResult
type ChunkResult struct {
	ChunkIndex   int       `json:"chunk_index"`
	Layer        int       `json:"layer"`
	Offset       int       `json:"offset,omitempty"`
	Category     []float32 `json:"category"`
	Target       []float32 `json:"target"`
	Err          error     `json:"-"`
	ErrorMessage string    `json:"error,omitempty"`
}

// FileInferenceResult агрегированный результат по всему файлу. Все чанки обработанные от ChunkResult
type FileInferenceResult struct {
	Filename string        `json:"filename"`
	Chunks   []ChunkResult `json:"chunks"`
}
