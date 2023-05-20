package parallel

import (
	"errors"
	"math/rand"
	"sync"
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
	input  chan inTaskWrapper[IN]
	output chan TaskResult

	mtx *sync.RWMutex
	wg  *sync.WaitGroup

	outChMap map[int64]chan TaskResult
}

func NewPool[IN any](threadCnt int, fn func(IN) (any, error)) *Pool[IN] {
	pool := &Pool[IN]{
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

func (p *Pool[IN]) resultRouter() {
	for o := range p.output {
		wrapper := o.(*taskResultWrapper)
		if ch, exists := p.outChMap[wrapper.id]; exists {
			ch <- o
		}
	}
}

func (p *Pool[IN]) Execute(data IN) TaskResult {
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

	result := <-p.outChMap[unique]
	close(p.outChMap[unique])
	delete(p.outChMap, unique)

	return result
}

func (p *Pool[IN]) Stop() {
	close(p.input)
	p.wg.Wait()
}
