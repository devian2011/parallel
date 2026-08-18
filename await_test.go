package parallel

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAwait_Success(t *testing.T) {
	ctx := context.Background()
	task := func(ctx context.Context) (string, error) {
		return "hello", nil
	}

	promise := Await(ctx, task)
	value, err := promise.Get()

	if value != "hello" {
		t.Errorf("expected 'hello', got '%s'", value)
	}
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestAwait_TaskReturnsError(t *testing.T) {
	ctx := context.Background()
	expectedErr := errors.New("task failed")
	task := func(ctx context.Context) (int, error) {
		return 0, expectedErr
	}

	promise := Await(ctx, task)
	value, err := promise.Get()

	if value != 0 {
		t.Errorf("expected zero value, got %d", value)
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestAwait_PanicRecovery(t *testing.T) {
	ctx := context.Background()
	task := func(ctx context.Context) (string, error) {
		panic("something went wrong")
	}

	promise := Await(ctx, task)
	_, err := promise.Get()

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrFnPanic) {
		t.Errorf("expected ErrFnPanic, got %v", err)
	}
}

func TestPromise_GetTwice(t *testing.T) {
	ctx := context.Background()
	task := func(ctx context.Context) (int, error) {
		return 42, nil
	}

	promise := Await(ctx, task)
	value1, err1 := promise.Get()
	value2, err2 := promise.Get()

	if value1 != 42 || value2 != 42 {
		t.Errorf("expected both results to be 42, got %d and %d", value1, value2)
	}
	if err1 != nil || err2 != nil {
		t.Errorf("expected no errors, got %v and %v", err1, err2)
	}
}

func TestAwait_TaskRespectsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	task := func(ctx context.Context) (string, error) {
		select {
		case <-time.After(100 * time.Millisecond):
			return "done", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	promise := Await(ctx, task)
	cancel()

	_, err := promise.Get()
	if err == nil {
		t.Error("expected error due to context cancellation, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestAwait_GetDoesNotBlockAfterCompletion(t *testing.T) {
	ctx := context.Background()
	task := func(ctx context.Context) (int, error) {
		return 100, nil
	}

	promise := Await(ctx, task)
	value, err := promise.Get()
	if value != 100 {
		t.Errorf("expected 100, got %d", value)
	}
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestAwait_ContextCanceledBeforeTaskFinishes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	task := func(ctx context.Context) (string, error) {
		time.Sleep(1 * time.Second)
		return "done", nil
	}

	promise := Await(ctx, task)
	cancel()

	_, err := promise.Get()
	if err == nil {
		t.Error("expected error due to context cancellation, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestAwait_ContextCanceledAfterTaskFinished(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	task := func(ctx context.Context) (int, error) {
		return 42, nil
	}

	promise := Await(ctx, task)
	value, err := promise.Get()
	if value != 42 || err != nil {
		t.Fatalf("expected 42, nil; got %d, %v", value, err)
	}

	cancel()
	value2, err2 := promise.Get()
	if value2 != 42 || err2 != nil {
		t.Errorf("expected cached 42, nil; got %d, %v", value2, err2)
	}
}

func TestAwait_GetAfterContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	task := func(ctx context.Context) (string, error) {
		time.Sleep(100 * time.Millisecond)
		return "ok", nil
	}

	promise := Await(ctx, task)
	cancel()

	_, err1 := promise.Get()
	if !errors.Is(err1, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err1)
	}

	_, err2 := promise.Get()
	if !errors.Is(err2, context.Canceled) {
		t.Errorf("expected context.Canceled on second call, got %v", err2)
	}
}

func TestAwait_PanicWithCustomType(t *testing.T) {
	type Custom struct {
		ID int
	}
	ctx := context.Background()
	task := func(ctx context.Context) (Custom, error) {
		panic("custom panic")
	}
	promise := Await(ctx, task)
	result, err := promise.Get()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrFnPanic) {
		t.Errorf("expected ErrFnPanic, got %v", err)
	}
	if result != (Custom{}) {
		t.Errorf("expected zero Custom, got %+v", result)
	}
}

func TestAwait_DeadlineExceeded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	task := func(ctx context.Context) (string, error) {
		select {
		case <-time.After(100 * time.Millisecond):
			return "done", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	promise := Await(ctx, task)
	_, err := promise.Get()
	if err == nil {
		t.Error("expected error due to timeout, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestAwait_GetDoesNotBlockAfterContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	task := func(ctx context.Context) (string, error) {
		time.Sleep(10 * time.Second)
		return "done", nil
	}
	promise := Await(ctx, task)
	cancel()
	start := time.Now()
	_, err := promise.Get()
	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Errorf("Get blocked too long after context cancel: %v", elapsed)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}
