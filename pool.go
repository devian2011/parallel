package parallel

import (
	"context"
	"errors"
	"math/rand"
	"sync"
)

var (
	ErrPoolIsStopped = errors.New("pool working is stopped")
)

type inTaskWrapper[IN any] struct {
	id    int64
	input IN
}

type taskResultWrapper[T any] struct {
	id     int64
	result TaskResult[T]
}

func (t *taskResultWrapper[T]) GetValue() T {
	return t.result.GetValue()
}

func (t *taskResultWrapper[T]) HasError() bool {
	return t.result.HasError()
}

func (t *taskResultWrapper[T]) GetError() error {
	return t.result.GetError()
}

type Pool[IN any, OUT any] struct {
	stopped bool

	input chan inTaskWrapper[IN]

	ctx context.Context
	mtx *sync.RWMutex
	wg  *sync.WaitGroup

	outChMap map[int64]chan TaskResult[OUT]
}

func NewPool[IN any, OUT any](ctx context.Context, threadCnt int, fn func(IN) (any, error)) *Pool[IN, OUT] {
	pool := &Pool[IN, OUT]{
		ctx:      ctx,
		stopped:  false,
		mtx:      &sync.RWMutex{},
		input:    make(chan inTaskWrapper[IN], threadCnt),
		outChMap: make(map[int64]chan TaskResult[OUT]),
	}

	pool.wg = HandleParallelOut[inTaskWrapper[IN]](
		threadCnt,
		pool.input,
		func(input inTaskWrapper[IN]) TaskResult[OUT] {
			out, err := fn(input.input)
			tr := &taskResultWrapper[OUT]{
				id: input.id,
			}
			if out != nil {
				tr.result = &TaskResultImpl[OUT]{
					value: out.(OUT),
					err:   err,
				}
			} else {
				tr.result = &TaskResultImpl[OUT]{
					err: err,
				}
			}

			return tr
		},
		func(result TaskResult[OUT]) {
			wrapper := result.(*taskResultWrapper[OUT])
			if ch, exists := pool.outChMap[wrapper.id]; exists {
				ch <- result
			}
		},
	)

	return pool
}

func (p *Pool[IN, OUT]) safeShutdown() {
	<-p.ctx.Done()
	p.Stop()
}

func (p *Pool[IN, OUT]) Execute(data IN) TaskResult[OUT] {
	if p.stopped {
		return &TaskResultImpl[OUT]{
			err: ErrPoolIsStopped,
		}
	}

	unique := rand.Int63()
	if _, exists := p.outChMap[unique]; exists {
		return &TaskResultImpl[OUT]{
			err: errors.New("ooh, it's strange but we have pool collision, please try your request later"),
		}
	}

	p.mtx.Lock()
	p.outChMap[unique] = make(chan TaskResult[OUT])
	p.mtx.Unlock()

	p.input <- inTaskWrapper[IN]{
		id:    unique,
		input: data,
	}

	select {
	case result := <-p.outChMap[unique]:
		close(p.outChMap[unique])
		delete(p.outChMap, unique)

		return result
	case <-p.ctx.Done():
		return &TaskResultImpl[OUT]{
			err: ErrPoolIsStopped,
		}
	}
}

func (p *Pool[IN, OUT]) Stop() {
	p.stopped = true
	close(p.input)
	p.wg.Wait()
}
