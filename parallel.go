package parallel

import (
	"context"
	"sync"
)

func HandleParallelChan[IN any](threadCnt int, input chan IN, fn func(IN) TaskResult) (chan TaskResult, *sync.WaitGroup) {
	wg := &sync.WaitGroup{}
	wg.Add(threadCnt)
	outputCh := make(chan TaskResult, threadCnt)
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

func HandleParallelOut[IN any](threadCnt int, input chan IN, fn func(IN) TaskResult, handler func(result TaskResult)) *sync.WaitGroup {
	outCh, wg := HandleParallelChan(threadCnt, input, fn)
	go func() {
		for o := range outCh {
			handler(o)
		}
	}()

	return wg
}

func HandleParallelArr[IN any](ctx context.Context, threadCnt int, input []IN, fn func(IN) TaskResult) []TaskResult {
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

	result := make([]TaskResult, len(input))
	go func() {
		for o := range out {
			result = append(result, o)
		}
	}()

	wg.Wait()
	return result
}
