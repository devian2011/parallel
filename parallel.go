package parallel

import (
	"context"
	"sync"
)

// HandleParallelChan process data from chan, and return output chan with wg
func HandleParallelChan[IN any, OUT any](threadCnt int, input chan IN, fn func(IN) TaskResult[OUT]) (chan TaskResult[OUT], *sync.WaitGroup) {
	wg := &sync.WaitGroup{}
	wg.Add(threadCnt)
	outputCh := make(chan TaskResult[OUT], threadCnt)
	for c := 0; c < threadCnt; c++ {
		go func() {
			for i := range input {
				outputCh <- fn(i)
			}
			wg.Done()
		}()
	}
	return outputCh, wg
}

// HandleParallelOut process data from input chan in many goroutines and send result to handler, return wait group
func HandleParallelOut[IN any, OUT any](threadCnt int, input chan IN, fn func(IN) TaskResult[OUT], handler func(result TaskResult[OUT])) *sync.WaitGroup {
	outCh, wg := HandleParallelChan(threadCnt, input, fn)
	wg.Add(1)
	go func() {
		for o := range outCh {
			handler(o)
		}
		wg.Done()
	}()

	return wg
}

// HandleParallelArr parallel processing of array
func HandleParallelArr[IN any, OUT any](ctx context.Context, threadCnt int, input []IN, fn func(IN) TaskResult[OUT]) []TaskResult[OUT] {
	inputCh := make(chan IN, len(input))
	go func() {
		<-ctx.Done()
		close(inputCh)
	}()
	go func() {
		for _, i := range input {
			inputCh <- i
		}
		close(inputCh)
	}()

	out, wg := HandleParallelChan(threadCnt, inputCh, fn)

	result := make([]TaskResult[OUT], len(input))
	go func() {
		for o := range out {
			result = append(result, o)
		}
	}()

	wg.Wait()
	return result
}
