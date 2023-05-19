package parallel

import (
	"context"
	"errors"
)

var (
	ErrAwaitBreaks = errors.New("await task breaks by context")
)

type Promise struct {
	ch  chan TaskResult
	ctx context.Context
}

func (p *Promise) run(task Task) {
	p.ch <- task()
}

func (p *Promise) Get() TaskResult {
	defer close(p.ch)

	select {
	case <-p.ctx.Done():
		return &TaskResultImpl{
			value: nil,
			err:   ErrAwaitBreaks,
		}
	case result := <-p.ch:
		return result
	}
}

func Await(ctx context.Context, task Task) *Promise {
	p := &Promise{
		ch:  make(chan TaskResult),
		ctx: ctx,
	}
	go p.run(task)
	return p
}
