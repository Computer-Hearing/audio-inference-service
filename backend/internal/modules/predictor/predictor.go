package predictor

import (
	"audio-inference-service/gen"
	"audio-inference-service/internal/domain"
	"audio-inference-service/internal/modules"
	"audio-inference-service/internal/modules/audio"
	"audio-inference-service/internal/modules/triton"
	"audio-inference-service/pkg"
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"time"

	"golang.org/x/sync/errgroup"
)

type Predictor struct {
	tritonConnector *triton.TritonClient
	taskManager     modules.TaskManager
	logger          *slog.Logger
}

type Options struct {
	TritonConnector *triton.TritonClient
	TaskManager     modules.TaskManager
	Logger          *slog.Logger
}

func New(opts *Options) *Predictor {
	if opts == nil {
		panic("predictor opts cannot be nil")
	}
	return &Predictor{
		tritonConnector: opts.TritonConnector,
		taskManager:     opts.TaskManager,
		logger:          opts.Logger,
	}
}

func (p *Predictor) ProcessTask(ctx context.Context, job domain.TaskPayload) error {
	taskID := job.TaskID

	modelName := job.Payload.ModelName
	if modelName == "" {
		return fmt.Errorf("model name is required")
	}

	// сам inference + замер времени всего инференса + вывод в лог этой инфы
	start := time.Now()
	result, inferErr := processAudioChunks(ctx, p.tritonConnector, modelName, job.Payload.Chunks)
	end := time.Since(start)

	logResult(p.logger, result, taskID.String(), end.String(), inferErr)

	// Обновляем статус в БД
	if inferErr != nil {
		if err := p.taskManager.StatusFailure(ctx, job.TaskID, result); err != nil {
			// TODO: плохо сделано, если была ошибка в инференсе и ошибка в бд, то выграет ошибка бд, хотя на нее тут в принципе похуй в отлчии от ошибки инференса
			return fmt.Errorf("set failure status: %w", err)
		}
		return fmt.Errorf("audio processing failed: %w", inferErr)
	}

	if err := p.taskManager.StatusSuccess(ctx, job.TaskID, result); err != nil {
		return fmt.Errorf("set success status: %w", err)
	}

	return nil
}

func processAudioChunks(
	ctx context.Context,
	client *triton.TritonClient,
	modelName string,
	audio audio.AudioChunks) (*domain.FileInferenceResult, error) {

	// сколько всего чанков у нас из слоев
	total := 0
	for _, layer := range audio.Layers {
		total += len(layer.Chunks)
	}

	results := make([]domain.ChunkResult, total)
	g, ctx := errgroup.WithContext(ctx)
	// ограничиваем количество параллельных запросов
	sem := make(chan struct{}, pkg.MaxTritonConcurrency)

	next := 0
	for li, layer := range audio.Layers {
		for ci, chunk := range layer.Chunks {
			cur := next
			next++
			sem <- struct{}{}
			g.Go(func() error {
				defer func() { <-sem }()
				results[cur] = processChunk(ctx, client, modelName, layer.Offset, li, ci, chunk)
				return nil // ошибки чанка не пробрасываем наверх, они уже в results[cur].Err
			})
		}
	}

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("unexpected error during chunk processing: %w", err)
	}

	return &domain.FileInferenceResult{
		Filename: audio.Filename,
		Chunks:   results,
	}, nil

}

// processChunk отправляет один чанк в Triton
func processChunk(
	ctx context.Context, client *triton.TritonClient,
	modelName string, offset, layer, index int, chunk []byte) domain.ChunkResult {

	result, err := runRawAudioInference(ctx, client, modelName, chunk)
	if err != nil {
		return domain.ChunkResult{
			ChunkIndex:   index,
			Layer:        layer,
			Offset:       offset,
			Err:          fmt.Errorf("inference failed: %w", err),
			ErrorMessage: err.Error(),
		}
	}

	return domain.ChunkResult{
		ChunkIndex: index,
		Layer:      layer,
		Offset:     offset,
		Category:   result.CategoryLogits,
		Target:     result.TargetLogits,
	}
}

// runRawAudioInference один запрос ModelInfer на один чанк
func runRawAudioInference(
	ctx context.Context,
	client *triton.TritonClient,
	modelName string,
	chunk []byte) (*domain.InferenceResult, error) {

	req := &gen.ModelInferRequest{
		ModelName: modelName,
		Inputs: []*gen.ModelInferRequest_InferInputTensor{
			{
				Name:     pkg.RawAudioInputName,
				Datatype: "UINT8",
				Shape:    []int64{1, int64(len(chunk))}, // max_batch_size:0 → без добавления батча
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

func parseResponse(resp *gen.ModelInferResponse) (*domain.InferenceResult, error) {
	result := &domain.InferenceResult{}

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

func logResult(logger *slog.Logger, chunks *domain.FileInferenceResult, taskID, inferenceTime string, inferenceErr error) {
	if inferenceErr != nil {
		logger.Error("inference error", "task_id", taskID, "error", inferenceErr.Error())
	}
	logger.Info("inference success", "task_id", taskID, "chunk len", len(chunks.Chunks), "inference time", inferenceTime, "filename", chunks.Filename)
}
