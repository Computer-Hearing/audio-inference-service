package predictor

import (
	"audio-inference-service/gen"
	"audio-inference-service/internal/chunks"
	"audio-inference-service/internal/domain"
	"audio-inference-service/internal/modules"
	"audio-inference-service/internal/modules/triton"
	"audio-inference-service/pkg"
	"context"
	"encoding/binary"
	"fmt"
	"golang.org/x/sync/errgroup"
	"math"
)

type Predictor struct {
	TritonConnector *triton.TritonClient
	TaskManager     modules.TaskManager[domain.AudioTaskPayload, chunks.FileInferenceResult]
}

func (p *Predictor) ProcessTask(ctx context.Context, job domain.TaskPayload[domain.AudioTaskPayload]) error {
	modelName := job.Payload.ModelName
	if modelName == "" {
		modelName = pkg.DefaultModelName
	}

	// База данных свободна, пока мы делаем долгий сетевой запрос к Тритону
	result, inferErr := processAudioChunks(ctx, p.TritonConnector, modelName, job.Payload.Chunks)

	// Обновляем статус в БД короткими транзакциями
	err := pkg.RetryDo(ctx, nil, func(ctx context.Context) error {
		if inferErr != nil {
			return p.TaskManager.StatusFailure(ctx, job.TaskID, result)
		}
		return p.TaskManager.StatusSuccess(ctx, job.TaskID, result)
	}, pkg.IsRetryableError)

	if err != nil {
		return fmt.Errorf("update task status: %w", err)
	}

	if inferErr != nil {
		return fmt.Errorf("audio processing failed: %w", inferErr)
	}

	return nil
}

func processAudioChunks(
	ctx context.Context,
	client *triton.TritonClient,
	modelName string,
	audio chunks.AudioChunks) (*chunks.FileInferenceResult, error) {

	results := make([]chunks.ChunkResult, len(audio.Chunks))
	g, ctx := errgroup.WithContext(ctx)
	sem := make(chan struct{}, pkg.MaxTritonConcurrency)

	for i, chunk := range audio.Chunks {
		i, chunk := i, chunk
		sem <- struct{}{}
		g.Go(func() error {
			defer func() { <-sem }()
			results[i] = processChunk(ctx, client, modelName, i, chunk)
			return nil // ошибки чанка не пробрасываем наверх, они уже в results[i].Err
		})
	}

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("unexpected error during chunk processing: %w", err)
	}

	return &chunks.FileInferenceResult{
		Filename: audio.Filename,
		Chunks:   results,
	}, nil

}

// processChunk отправляет один чанк в Triton
func processChunk(ctx context.Context, client *triton.TritonClient, modelName string, index int, chunk []byte) chunks.ChunkResult {
	result, err := runRawAudioInference(ctx, client, modelName, chunk)
	if err != nil {
		return chunks.ChunkResult{
			ChunkIndex:   index,
			Err:          fmt.Errorf("inference failed: %w", err),
			ErrorMessage: err.Error(),
		}
	}

	return chunks.ChunkResult{
		ChunkIndex: index,
		Category:   result.CategoryLogits,
		Target:     result.TargetLogits,
	}
}

// runRawAudioInference один запрос ModelInfer на один чанк
func runRawAudioInference(
	ctx context.Context,
	client *triton.TritonClient,
	modelName string,
	chunk []byte) (*chunks.InferenceResult, error) {

	req := &gen.ModelInferRequest{
		ModelName: modelName,
		Inputs: []*gen.ModelInferRequest_InferInputTensor{
			{
				Name:     pkg.RawAudioInputName,
				Datatype: "UINT8",
				Shape:    []int64{int64(len(chunk))}, // max_batch_size:0 → без добавления батча
			},
		},
		Outputs: []*gen.ModelInferRequest_InferRequestedOutputTensor{
			{Name: pkg.CategoryOutputName},
			{Name: pkg.TargetOutputName},
		},
		RawInputContents: [][]byte{chunk},
	}

	resp, err := client.Infer(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("model infer failed: %w", err)
	}

	return parseResponse(resp)
}

func parseResponse(resp *gen.ModelInferResponse) (*chunks.InferenceResult, error) {
	result := &chunks.InferenceResult{}

	for i, out := range resp.Outputs {

		if i >= len(resp.RawOutputContents) {
			return nil, fmt.Errorf("missing raw content for output %s", out.Name)
		}

		values := bytesToFloat32(resp.RawOutputContents[i])

		switch out.Name {
		case pkg.CategoryOutputName:
			if len(values) != 5 {
				return nil, fmt.Errorf("unexpected category_output size: %d", len(values))
			}
			result.CategoryLogits = values
		case pkg.TargetOutputName:
			if len(values) != 7 {
				return nil, fmt.Errorf("unexpected target_output size: %d", len(values))
			}
			result.TargetLogits = values
		default:
			return nil, fmt.Errorf("unexpected output name: %s", out.Name)
		}
	}

	if result.CategoryLogits == nil || result.TargetLogits == nil {
		return nil, fmt.Errorf("incomplete response: missing one or more expected outputs")
	}

	return result, nil
}

func bytesToFloat32(raw []byte) []float32 {
	result := make([]float32, len(raw)/4)

	for i := range result {
		bits := binary.LittleEndian.Uint32(raw[i*4 : i*4+4])
		result[i] = math.Float32frombits(bits)
	}

	return result
}
