package parallel

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAwait_Success(t *testing.T) {
	ctx := context.Background()
	task := func(ctx context.Context) TaskResult[string] {
		return TaskResult[string]{Value: "hello", Err: nil}
	}

	promise := Await(ctx, task)
	result := promise.Get()

	if result.Value != "hello" {
		t.Errorf("expected 'hello', got '%s'", result.Value)
	}
	if result.Err != nil {
		t.Errorf("expected no error, got %v", result.Err)
	}
}

func TestAwait_TaskReturnsError(t *testing.T) {
	ctx := context.Background()
	expectedErr := errors.New("task failed")
	task := func(ctx context.Context) TaskResult[int] {
		return TaskResult[int]{Err: expectedErr}
	}

	promise := Await(ctx, task)
	result := promise.Get()

	if result.Value != 0 {
		t.Errorf("expected zero value, got %d", result.Value)
	}
	if !errors.Is(result.Err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, result.Err)
	}
}

func TestAwait_PanicRecovery(t *testing.T) {
	ctx := context.Background()
	task := func(ctx context.Context) TaskResult[string] {
		panic("something went wrong")
	}

	promise := Await(ctx, task)
	result := promise.Get()

	if result.Err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(result.Err, ErrAwaitPanic) {
		t.Errorf("expected ErrAwaitPanic, got %v", result.Err)
	}
	// Check that the error contains the panic message.
	if !errors.Is(result.Err, ErrAwaitPanic) {
		t.Errorf("error should wrap ErrAwaitPanic")
	}
}

func TestPromise_GetTwice(t *testing.T) {
	ctx := context.Background()
	task := func(ctx context.Context) TaskResult[int] {
		return TaskResult[int]{Value: 42, Err: nil}
	}

	promise := Await(ctx, task)
	result1 := promise.Get()
	result2 := promise.Get()

	if result1.Value != 42 || result2.Value != 42 {
		t.Errorf("expected both results to be 42, got %d and %d", result1.Value, result2.Value)
	}
	if result1.Err != nil || result2.Err != nil {
		t.Errorf("expected no errors, got %v and %v", result1.Err, result2.Err)
	}
}

func TestAwait_TaskRespectsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	task := func(ctx context.Context) TaskResult[string] {
		select {
		case <-time.After(100 * time.Millisecond):
			return TaskResult[string]{Value: "done"}
		case <-ctx.Done():
			return TaskResult[string]{Err: ctx.Err()}
		}
	}

	promise := Await(ctx, task)
	cancel() // cancel immediately

	result := promise.Get()
	if result.Err == nil {
		t.Error("expected error due to context cancellation, got nil")
	}
	if !errors.Is(result.Err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", result.Err)
	}
}

func TestAwait_GetDoesNotBlockAfterCompletion(t *testing.T) {
	ctx := context.Background()
	task := func(ctx context.Context) TaskResult[int] {
		return TaskResult[int]{Value: 100}
	}

	promise := Await(ctx, task)
	// Wait a tiny bit to ensure the task completes.
	time.Sleep(10 * time.Millisecond)
	result := promise.Get()
	if result.Value != 100 {
		t.Errorf("expected 100, got %d", result.Value)
	}
}
