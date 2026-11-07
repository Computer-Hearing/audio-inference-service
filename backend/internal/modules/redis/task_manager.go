// Package redis хранит состояние задач и доставляет их воркерам через Redis.
//
// Схема:
//   - task:<id>          — hash с состоянием (status, model, attempts, result);
//   - task:<id>:payload  — JSON с чанками аудио, удаляется после завершения;
//   - stream tasks       — очередь задач, в сообщении только task_id.
//
// Все ключи живут TASK_TTL и обновляются при каждом изменении, поэтому
// завершённые задачи не накапливаются.
package redis

import (
	"audio-inference-service/internal/domain"
	"audio-inference-service/internal/domain/dto"
	"audio-inference-service/pkg"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const (
	fieldStatus   = "status"
	fieldModel    = "model"
	fieldAttempts = "attempts"
	fieldResult   = "result"

	stateKeyPrefix   = "task:"
	payloadKeySuffix = ":payload"

	// streamFieldTaskID — имя поля в сообщении очереди.
	streamFieldTaskID = "task_id"

	// streamMaxLen ограничивает размер стрима: сообщения после ack больше не нужны.
	streamMaxLen = 10000
)

// markProcessingScript атомарно переводит задачу в processing и увеличивает
// счётчик попыток. Если задачи уже нет (истёк TTL) — возвращает -1 и не создаёт
// новый ключ. Если задача уже завершена — возвращает -2: повторять инференс не
// нужно, достаточно подтвердить сообщение.
var markProcessingScript = goredis.NewScript(`
local status = redis.call('HGET', KEYS[1], 'status')
if not status then
  return -1
end
if status == ARGV[3] or status == ARGV[4] then
  redis.call('EXPIRE', KEYS[1], ARGV[2])
  return -2
end
redis.call('HSET', KEYS[1], 'status', ARGV[1])
local attempts = redis.call('HINCRBY', KEYS[1], 'attempts', 1)
redis.call('EXPIRE', KEYS[1], ARGV[2])
redis.call('EXPIRE', KEYS[2], ARGV[2])
return attempts
`)

// updateStatusScript записывает финальный статус и удаляет payload.
// Если задачи уже нет — возвращает 0.
var updateStatusScript = goredis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then
  return 0
end
redis.call('HSET', KEYS[1], 'status', ARGV[1], 'result', ARGV[2])
redis.call('EXPIRE', KEYS[1], ARGV[3])
redis.call('DEL', KEYS[2])
return 1
`)

// Options параметры Redis-хранилища задач.
type Options struct {
	Client   *goredis.Client
	Stream   string
	Group    string
	Consumer string
	TaskTTL  time.Duration
}

// TaskManager реализует modules.TaskManager и modules.TaskQueue поверх Redis.
type TaskManager struct {
	client   *goredis.Client
	stream   string
	group    string
	consumer string
	ttl      time.Duration
}

func New(opts *Options) *TaskManager {
	if opts == nil || opts.Client == nil {
		panic("redis task manager: client is required")
	}
	if opts.Stream == "" {
		opts.Stream = "tasks"
	}
	if opts.Group == "" {
		opts.Group = "inference"
	}
	if opts.Consumer == "" {
		opts.Consumer = "backend"
	}
	if opts.TaskTTL <= 0 {
		opts.TaskTTL = time.Hour
	}

	return &TaskManager{
		client:   opts.Client,
		stream:   opts.Stream,
		group:    opts.Group,
		consumer: opts.Consumer,
		ttl:      opts.TaskTTL,
	}
}

func stateKey(taskID domain.TaskID) string {
	return stateKeyPrefix + string(taskID)
}

func payloadKey(taskID domain.TaskID) string {
	return stateKey(taskID) + payloadKeySuffix
}

func (m *TaskManager) ttlSeconds() int64 {
	seconds := int64(m.ttl / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	return seconds
}

// GetTask возвращает состояние задачи без чанков
func (m *TaskManager) GetTask(ctx context.Context, taskID domain.TaskID) (*domain.Task, error) {
	if err := taskID.IsValid(); err != nil {
		return nil, err
	}

	fields, err := m.client.HGetAll(ctx, stateKey(taskID)).Result()
	if err != nil {
		return nil, pkg.NewInternalWithDetails(
			"redis error while getting task",
			map[string]string{"error": err.Error()},
		)
	}
	if len(fields) == 0 {
		return nil, pkg.NewNotFoundWithDetails(
			"Task not found",
			map[string]string{"taskID": string(taskID)},
		)
	}

	task := &domain.Task{
		TaskID: taskID,
		Status: dto.TaskStatusFromString(fields[fieldStatus]),
		Model:  fields[fieldModel],
	}

	if raw := fields[fieldResult]; raw != "" {
		var result domain.FileInferenceResult
		if err := json.Unmarshal([]byte(raw), &result); err != nil {
			return nil, pkg.NewInternalWithDetails(
				"failed to unmarshal task result",
				map[string]string{"taskID": string(taskID), "error": err.Error()},
			)
		}
		task.Result = &result
	}

	return task, nil
}

// CreateTask сохраняет состояние и payload, а также ставит задачу в очередь.
// Все операции выполняются в одной транзакции: задачи-сироты без сообщения
// в очереди не появится.
func (m *TaskManager) CreateTask(
	ctx context.Context, taskID domain.TaskID, payload domain.AudioTaskPayload) error {

	details := make(map[string]string)
	if string(taskID) == "" {
		details["taskID"] = "cannot be empty"
	}
	if len(payload.Chunks.Layers) == 0 {
		details["chunks"] = "must contain at least one audio chunk"
	}
	if payload.ModelName == "" {
		details["model"] = "cannot be empty"
	}
	if len(details) > 0 {
		return pkg.NewBadRequestWithDetails("create task validation failed", details)
	}

	chunksJSON, err := json.Marshal(payload.Chunks)
	if err != nil {
		return pkg.NewInternalWithDetails(
			"marshal audio chunks failed",
			map[string]string{"error": err.Error()},
		)
	}

	_, err = m.client.TxPipelined(ctx, func(pipe goredis.Pipeliner) error {
		pipe.HSet(ctx, stateKey(taskID),
			fieldStatus, dto.TaskStatusToString(domain.TaskStatusPending),
			fieldModel, payload.ModelName,
			fieldAttempts, 0,
		)
		pipe.Expire(ctx, stateKey(taskID), m.ttl)
		pipe.Set(ctx, payloadKey(taskID), chunksJSON, m.ttl)
		pipe.XAdd(ctx, &goredis.XAddArgs{
			Stream: m.stream,
			MaxLen: streamMaxLen,
			Approx: true,
			Values: map[string]any{streamFieldTaskID: string(taskID)},
		})
		return nil
	})
	if err != nil {
		return pkg.NewInternalWithDetails(
			"create task error",
			map[string]string{"error": err.Error()},
		)
	}

	return nil
}

// GetPayload возвращает данные задачи для воркера.
// Второе значение — false, если задача уже истекла или завершилась.
func (m *TaskManager) GetPayload(
	ctx context.Context, taskID domain.TaskID) (*domain.AudioTaskPayload, bool, error) {

	raw, err := m.client.Get(ctx, payloadKey(taskID)).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, false, nil
		}
		return nil, false, pkg.NewInternalWithDetails(
			"redis error while getting task payload",
			map[string]string{"error": err.Error()},
		)
	}

	var payload domain.AudioTaskPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, false, pkg.NewInternalWithDetails(
			"failed to unmarshal task payload",
			map[string]string{"taskID": string(taskID), "error": err.Error()},
		)
	}

	return &payload, true, nil
}

// MarkProcessing переводит задачу в processing и увеличивает счётчик попыток.
// Второе значение — false, если задачи уже нет.
func (m *TaskManager) MarkProcessing(
	ctx context.Context, taskID domain.TaskID) (int, bool, error) {

	if err := taskID.IsValid(); err != nil {
		return 0, false, err
	}

	attempts, err := markProcessingScript.Run(ctx, m.client,
		[]string{stateKey(taskID), payloadKey(taskID)},
		dto.TaskStatusToString(domain.TaskStatusProcessing),
		m.ttlSeconds(),
		dto.TaskStatusToString(domain.TaskStatusSuccess),
		dto.TaskStatusToString(domain.TaskStatusFailure),
	).Int64()
	if err != nil {
		return 0, false, pkg.NewInternalWithDetails(
			"redis error while marking task processing",
			map[string]string{"error": err.Error()},
		)
	}
	if attempts < 0 {
		return 0, false, nil
	}

	return int(attempts), true, nil
}

// StatusSuccess сохраняет успешный результат и удаляет payload.
func (m *TaskManager) StatusSuccess(
	ctx context.Context, taskID domain.TaskID, result *domain.FileInferenceResult) error {

	if string(taskID) == "" {
		return pkg.NewBadRequestWithDetails(
			"validation failed",
			map[string]string{"taskID": "cannot be empty"},
		)
	}
	if result == nil {
		return pkg.NewBadRequestWithDetails(
			"validation failed",
			map[string]string{"result": "cannot be nil"},
		)
	}

	raw, err := json.Marshal(result)
	if err != nil {
		return pkg.NewInternalWithDetails(
			"marshal result data error",
			map[string]string{"error": err.Error()},
		)
	}

	return m.updateStatus(ctx, taskID, domain.TaskStatusSuccess, string(raw))
}

// StatusFailure сохраняет статус ошибки и частичный результат, если он есть.
func (m *TaskManager) StatusFailure(
	ctx context.Context, taskID domain.TaskID, result *domain.FileInferenceResult) error {

	if string(taskID) == "" {
		return pkg.NewBadRequestWithDetails(
			"validation failed",
			map[string]string{"taskID": "cannot be empty"},
		)
	}

	resultJSON := ""
	if result != nil {
		raw, err := json.Marshal(result)
		if err != nil {
			return pkg.NewInternalWithDetails(
				"failed to marshal partial result data",
				map[string]string{"error": err.Error()},
			)
		}
		resultJSON = string(raw)
	}

	return m.updateStatus(ctx, taskID, domain.TaskStatusFailure, resultJSON)
}

func (m *TaskManager) updateStatus(
	ctx context.Context, taskID domain.TaskID, status domain.TaskStatus, resultJSON string) error {

	updated, err := updateStatusScript.Run(ctx, m.client,
		[]string{stateKey(taskID), payloadKey(taskID)},
		dto.TaskStatusToString(status),
		resultJSON,
		m.ttlSeconds(),
	).Int64()
	if err != nil {
		return pkg.NewInternalWithDetails(
			"set status error",
			map[string]string{"error": err.Error()},
		)
	}
	if updated == 0 {
		return pkg.NewNotFoundWithDetails(
			"Task not found",
			map[string]string{"taskID": string(taskID)},
		)
	}

	return nil
}

// EnsureGroup создаёт consumer group, если её ещё нет.
func (m *TaskManager) EnsureGroup(ctx context.Context) error {
	err := m.client.XGroupCreateMkStream(ctx, m.stream, m.group, "$").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return fmt.Errorf("create consumer group: %w", err)
	}
	return nil
}

// Read забирает новые сообщения из очереди.
func (m *TaskManager) Read(
	ctx context.Context, count int, block time.Duration) ([]domain.TaskDelivery, error) {

	res, err := m.client.XReadGroup(ctx, &goredis.XReadGroupArgs{
		Group:    m.group,
		Consumer: m.consumer,
		Streams:  []string{m.stream, ">"},
		Count:    int64(count),
		Block:    block,
	}).Result()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, nil
		}
		return nil, fmt.Errorf("read task stream: %w", err)
	}

	var deliveries []domain.TaskDelivery
	for _, stream := range res {
		for _, msg := range stream.Messages {
			delivery, ok := m.parseDelivery(msg)
			if !ok {
				// Битое сообщение подтверждаем, чтобы оно не висело в PEL.
				_ = m.client.XAck(ctx, m.stream, m.group, msg.ID).Err()
				continue
			}
			deliveries = append(deliveries, delivery)
		}
	}

	return deliveries, nil
}

// Reclaim забирает сообщения, которые не были подтверждены дольше minIdle.
// Переотправка зависших processing-задач.
func (m *TaskManager) Reclaim(
	ctx context.Context, minIdle time.Duration, count int) ([]domain.TaskDelivery, error) {

	msgs, _, err := m.client.XAutoClaim(ctx, &goredis.XAutoClaimArgs{
		Stream:   m.stream,
		Group:    m.group,
		Consumer: m.consumer,
		MinIdle:  minIdle,
		Start:    "0-0",
		Count:    int64(count),
	}).Result()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, nil
		}
		return nil, fmt.Errorf("reclaim task stream: %w", err)
	}

	var deliveries []domain.TaskDelivery
	for _, msg := range msgs {
		delivery, ok := m.parseDelivery(msg)
		if !ok {
			_ = m.client.XAck(ctx, m.stream, m.group, msg.ID).Err()
			continue
		}
		deliveries = append(deliveries, delivery)
	}

	return deliveries, nil
}

// Ack подтверждает обработку сообщения.
func (m *TaskManager) Ack(ctx context.Context, receipt string) error {
	if err := m.client.XAck(ctx, m.stream, m.group, receipt).Err(); err != nil {
		return fmt.Errorf("ack task message: %w", err)
	}
	return nil
}

func (m *TaskManager) parseDelivery(msg goredis.XMessage) (domain.TaskDelivery, bool) {
	raw, ok := msg.Values[streamFieldTaskID]
	if !ok {
		return domain.TaskDelivery{}, false
	}

	taskID, ok := raw.(string)
	if !ok || taskID == "" {
		return domain.TaskDelivery{}, false
	}

	return domain.TaskDelivery{
		TaskID:  domain.TaskID(taskID),
		Receipt: msg.ID,
	}, true
}
