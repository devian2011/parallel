package parallel

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestHandleParallelChan(t *testing.T) {
	ctx := context.Background()
	input := make(chan int, 10)
	for i := 0; i < 10; i++ {
		input <- i
	}
	close(input)

	fn := func(ctx context.Context, n int) TaskResult[string] {
		return TaskResult[string]{Value: strconv.Itoa(n), Err: nil}
	}

	outCh, err := HandleParallelChan(ctx, 3, input, fn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	results := make([]string, 0, 10)
	for result := range outCh {
		results = append(results, result.Value)
	}

	if len(results) != 10 {
		t.Errorf("expected 10 results, got %d", len(results))
	}
}

func TestHandleParallelOut(t *testing.T) {
	ctx := context.Background()
	input := make(chan int, 5)
	for i := 0; i < 5; i++ {
		input <- i
	}
	close(input)

	collected := make([]string, 0, 5)
	var mu sync.Mutex

	fn := func(ctx context.Context, n int) TaskResult[string] {
		return TaskResult[string]{Value: strconv.Itoa(n)}
	}
	handler := func(ctx context.Context, res TaskResult[string]) {
		mu.Lock()
		collected = append(collected, res.Value)
		mu.Unlock()
	}

	wg, err := HandleParallelOut(ctx, 2, input, fn, handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(collected) != 5 {
		t.Errorf("expected 5 results, got %d", len(collected))
	}
}

func TestHandleParallelArr(t *testing.T) {
	ctx := context.Background()
	input := []int{1, 2, 3, 4, 5}
	fn := func(ctx context.Context, n int) TaskResult[int] {
		return TaskResult[int]{Value: n * 10}
	}

	results, err := HandleParallelArr(ctx, 3, input, fn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != len(input) {
		t.Errorf("expected %d results, got %d", len(input), len(results))
	}
	for i, r := range results {
		if r.Value != input[i]*10 {
			t.Errorf("index %d: expected %d, got %d", i, input[i]*10, r.Value)
		}
	}
}

func TestHandleParallelArrWithError(t *testing.T) {
	ctx := context.Background()
	input := []int{1, 2, 3}
	fn := func(ctx context.Context, n int) TaskResult[int] {
		if n == 2 {
			return TaskResult[int]{Err: errors.New("error on 2")}
		}
		return TaskResult[int]{Value: n}
	}

	results, err := HandleParallelArr(ctx, 2, input, fn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Value != 1 || results[0].Err != nil {
		t.Errorf("first result mismatch")
	}
	if results[1].Err == nil || results[1].Err.Error() != "error on 2" {
		t.Errorf("second error expected")
	}
	if results[2].Value != 3 || results[2].Err != nil {
		t.Errorf("third result mismatch")
	}
}

func TestHandleParallelArrContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	input := []int{1, 2, 3, 4, 5}
	fn := func(ctx context.Context, n int) TaskResult[int] {
		time.Sleep(10 * time.Millisecond)
		return TaskResult[int]{Value: n}
	}

	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	results, err := HandleParallelArr(ctx, 10, input, fn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != len(input) {
		t.Errorf("expected len %d, got %d", len(input), len(results))
	}
	// Some results may be zero values if tasks were cancelled.
	for _, r := range results {
		if r.Value == 0 && r.Err == nil {
			// possible if task was cancelled before execution – that's fine
		}
	}
}

func TestHandleParallelArrEmptyInput(t *testing.T) {
	ctx := context.Background()
	fn := func(ctx context.Context, n int) TaskResult[int] { return TaskResult[int]{Value: n} }
	results, err := HandleParallelArr(ctx, 5, []int{}, fn)
	if err == nil {
		t.Errorf("expected error for empty input, got nil")
	}
	if results != nil {
		t.Errorf("expected nil slice, got %v", results)
	}
}

func TestHandleParallelArrInvalidThreadCnt(t *testing.T) {
	ctx := context.Background()
	_, err := HandleParallelArr(ctx, 0, []int{1, 2}, func(ctx context.Context, n int) TaskResult[int] {
		return TaskResult[int]{Value: n}
	})
	if err == nil {
		t.Error("expected error for threadCnt <= 0")
	}
}
