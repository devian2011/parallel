package parallel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
)

func recoverWrapper[IN any, OUT any](
	ctx context.Context,
	in IN,
	fn func(context.Context, IN) TaskResult[OUT],
) (result TaskResult[OUT]) {
	defer func() {
		if err := recover(); err != nil {
			result.Value = *new(OUT)
			result.Err = errors.Join(ErrFnPanic, errors.New(err.(string)))
		}
	}()
	return fn(ctx, in)
}

// HandleParallelChan reads from input channel, processes each item with fn
// using up to threadCnt concurrent goroutines, and returns a buffered output
// channel of TaskResult[OUT]. The caller must close the input channel when
// no more items will be sent. The returned WaitGroup can be used to wait
// for all workers to finish. The output channel is closed after all workers
// have finished, so callers can range over it.
func HandleParallelChan[IN any, OUT any](
	ctx context.Context,
	threadCnt int,
	input <-chan IN,
	fn func(context.Context, IN) TaskResult[OUT],
) (<-chan TaskResult[OUT], error) {
	if input == nil {
		return nil, fmt.Errorf("empty input")
	}
	if threadCnt <= 0 {
		return nil, fmt.Errorf("threadCnt must be greater than 0")
	}

	wg := &sync.WaitGroup{}
	wg.Add(threadCnt)
	outputCh := make(chan TaskResult[OUT], threadCnt)

	for c := 0; c < threadCnt; c++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case in, ok := <-input:
					if !ok {
						return
					}

					select {
					case <-ctx.Done():
						return
					case outputCh <- recoverWrapper(ctx, in, fn):
					}
				}
			}
		}()
	}

	// Close output channel after all workers finish so that consumers
	// (like the handler goroutine) can exit.
	go func() {
		wg.Wait()
		close(outputCh)
	}()

	return outputCh, nil
}

// HandleParallelOut processes items from input channel using threadCnt workers,
// and sends each result to the provided handler function (e.g., for side effects
// like saving to a database or aggregating). The caller is responsible for
// closing the input channel after all items are sent.
// It returns a single WaitGroup that will be marked done when both the workers
// have finished processing all items AND the handler has processed all results.
func HandleParallelOut[IN any, OUT any](
	ctx context.Context,
	threadCnt int,
	input <-chan IN,
	fn func(context.Context, IN) TaskResult[OUT],
	handler func(ctx context.Context, result TaskResult[OUT]),
) (*sync.WaitGroup, error) {
	if input == nil {
		return nil, fmt.Errorf("empty input")
	}
	if threadCnt <= 0 {
		return nil, fmt.Errorf("threadCnt must be greater than 0")
	}

	outCh, err := HandleParallelChan(ctx, threadCnt, input, fn)
	if err != nil {
		return nil, err
	}

	wg := &sync.WaitGroup{}
	wg.Add(threadCnt)
	for c := 0; c < threadCnt; c++ {
		go func() {
			defer wg.Done()
			for result := range outCh {
				func() {
					defer func() {
						if err := recover(); err != nil {
							slog.Error("error parallel out handler", "err", err)
						}
					}()
					handler(ctx, result)
				}()

			}
		}()
	}

	return wg, nil
}

// HandleParallelArr processes a slice of items concurrently, using up to
// threadCnt workers. The context ctx allows cancellation: if cancelled,
// the function returns early with the partial results already computed.
// The returned slice preserves the original order of the input.
// If the input slice is empty, it returns nil.
func HandleParallelArr[IN any, OUT any](
	ctx context.Context,
	threadCnt int,
	input []IN,
	fn func(context.Context, IN) TaskResult[OUT],
) ([]TaskResult[OUT], error) {
	if len(input) == 0 {
		return nil, fmt.Errorf("empty input")
	}
	if threadCnt <= 0 {
		return nil, fmt.Errorf("threadCnt must be greater than 0")
	}

	type workItem struct {
		idx  int
		item IN
	}

	workCh := make(chan workItem, len(input))
	go func() {
		defer close(workCh)
		for idx, item := range input {
			select {
			case <-ctx.Done():
				return
			case workCh <- workItem{idx: idx, item: item}:
			}
		}
	}()

	type resultItem struct {
		idx    int
		result TaskResult[OUT]
	}
	resCh := make(chan resultItem, len(input))

	wg := &sync.WaitGroup{}
	wg.Add(threadCnt)
	for c := 0; c < threadCnt; c++ {
		go func() {
			defer wg.Done()
			for wi := range workCh {
				select {
				case <-ctx.Done():
					return
				default:
					resCh <- resultItem{
						idx:    wi.idx,
						result: recoverWrapper(ctx, wi.item, fn),
					}
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(resCh)
	}()

	results := make([]TaskResult[OUT], len(input))
	for ri := range resCh {
		results[ri.idx] = ri.result
	}
	return results, nil
}
