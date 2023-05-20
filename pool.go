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

type taskResultWrapper struct {
	id     int64
	result TaskResult
}

func (t *taskResultWrapper) GetValue() any {
	return t.result.GetValue()
}

func (t *taskResultWrapper) HasError() bool {
	return t.result.HasError()
}

func (t *taskResultWrapper) GetError() error {
	return t.result.GetError()
}

type Pool[IN any] struct {
	stopped bool

	input  chan inTaskWrapper[IN]
	output chan TaskResult

	ctx context.Context
	mtx *sync.RWMutex
	wg  *sync.WaitGroup

	outChMap map[int64]chan TaskResult
}

func NewPool[IN any](ctx context.Context, threadCnt int, fn func(IN) (any, error)) *Pool[IN] {
	pool := &Pool[IN]{
		ctx:      ctx,
		stopped:  false,
		mtx:      &sync.RWMutex{},
		input:    make(chan inTaskWrapper[IN], threadCnt),
		output:   make(chan TaskResult, threadCnt),
		outChMap: make(map[int64]chan TaskResult),
	}

	pool.output, pool.wg = HandleParallelChan[inTaskWrapper[IN]](threadCnt, pool.input, func(input inTaskWrapper[IN]) TaskResult {
		out, err := fn(input.input)
		return &taskResultWrapper{
			result: &TaskResultImpl{
				value: out,
				err:   err,
			},
			id: input.id,
		}
	})

	go pool.resultRouter()

	return pool
}

func (p *Pool[IN]) safeShutdown() {
	<-p.ctx.Done()
	p.Stop()
}

func (p *Pool[IN]) resultRouter() {
	for o := range p.output {
		wrapper := o.(*taskResultWrapper)
		if ch, exists := p.outChMap[wrapper.id]; exists {
			ch <- o
		}
	}
}

func (p *Pool[IN]) Execute(data IN) TaskResult {
	if p.stopped {
		return &TaskResultImpl{
			value: nil,
			err:   ErrPoolIsStopped,
		}
	}

	unique := rand.Int63()
	if _, exists := p.outChMap[unique]; exists {
		return &TaskResultImpl{
			value: nil,
			err:   errors.New("ooh, it's strange but we have pool collision, please try your request later"),
		}
	}

	p.mtx.Lock()
	p.outChMap[unique] = make(chan TaskResult)
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
		return &TaskResultImpl{
			value: nil,
			err:   ErrPoolIsStopped,
		}
	}
}

func (p *Pool[IN]) Stop() {
	p.stopped = true
	close(p.input)
	p.wg.Wait()
}
