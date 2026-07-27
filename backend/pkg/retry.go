package pkg

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgconn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"math/rand"
	"time"
)

// Config настройки retry
type Config struct {
	MaxRetries    int           // максимум попыток (не считая первую)
	InitialDelay  time.Duration // задержка перед первым повтором
	MaxDelay      time.Duration // потолок задержки
	Multiplier    float64       // во сколько раз растёт задержка
	Jitter        bool          // добавлять случайный разброс, чтобы избежать "thundering herd"
	JitterPercent float64       // на сколько % может отклониться задержка (например 0.2 = ±20%)
}

// DefaultConfig разумные значения по умолчанию
func DefaultConfig() Config {
	return Config{
		MaxRetries:    5,
		InitialDelay:  500 * time.Millisecond,
		MaxDelay:      30 * time.Second,
		Multiplier:    2.0,
		Jitter:        true,
		JitterPercent: 0.2,
	}
}

// RetryableFunc функция, которую нужно повторять
type RetryableFunc func(ctx context.Context) error

// IsRetryableFunc определяет, стоит ли повторять при данной ошибке
// (например, не имеет смысла ретраить ошибку валидации)
type IsRetryableFunc func(err error) bool

// RetryDo выполняет fn с экспоненциальным backoff.
// Останавливается при успехе, при исчерпании попыток, при отмене контекста
// или если IsRetryableFunc вернула false для конкретной ошибки.
func RetryDo(ctx context.Context, cfg *Config, fn RetryableFunc, isRetryable IsRetryableFunc) error {
	if cfg == nil {
		*cfg = DefaultConfig()
	}

	if isRetryable == nil {
		isRetryable = func(error) bool { return true } // по умолчанию ретраим всё
	}

	delay := cfg.InitialDelay
	var lastErr error

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context cancelled before attempt %d: %w", attempt, err)
		}

		err := fn(ctx)
		if err == nil {
			return nil // успех
		}

		lastErr = err

		if !isRetryable(err) {
			return fmt.Errorf("non-retryable error on attempt %d: %w", attempt, err)
		}

		if attempt == cfg.MaxRetries {
			break // последняя попытка исчерпана
		}

		wait := delay
		if cfg.Jitter {
			wait = applyJitter(delay, cfg.JitterPercent)
		}

		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during backoff: %w", ctx.Err())
		}

		// увеличиваем задержку, не превышая потолок
		delay = time.Duration(float64(delay) * cfg.Multiplier)
		if delay > cfg.MaxDelay {
			delay = cfg.MaxDelay
		}
	}

	return fmt.Errorf("all %d attempts failed, last error: %w", cfg.MaxRetries+1, lastErr)
}

// applyJitter добавляет случайное отклонение к задержке в пределах ±percent
func applyJitter(d time.Duration, percent float64) time.Duration {
	if percent <= 0 {
		return d
	}
	delta := float64(d) * percent
	// случайное значение в диапазоне [-delta, +delta]
	offset := (rand.Float64()*2 - 1) * delta
	result := time.Duration(float64(d) + offset)
	if result < 0 {
		return 0
	}
	return result
}

// IsRetryableError решает, какие gRPC-ошибки имеет смысл повторять
func IsRetryableError(err error) bool {
	st, ok := status.FromError(err)
	if ok {
		switch st.Code() {
		case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted, codes.Aborted:
			return true // временные проблемы — стоит повторить
		case codes.InvalidArgument, codes.NotFound, codes.PermissionDenied, codes.Unauthenticated:
			return false // ошибки клиента — повтор не поможет
		default:
			return true
		}
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "40001", "40P01": // serialization_failure, deadlock_detected
			return true
		default:
			return false
		}
	}

	return true
}
