package parallel

import (
	"context"
	"errors"
)

var (
	ErrAwaitBreaks = errors.New("await task breaks by context")
)

type Promise[T any] struct {
	ch  chan TaskResult[T]
	ctx context.Context
}

func (p *Promise[T]) run(task Task[T]) {
	p.ch <- task()
}

func (p *Promise[T]) Get() TaskResult[T] {
	defer close(p.ch)

	select {
	case <-p.ctx.Done():
		return &TaskResultImpl[T]{
			err: ErrAwaitBreaks,
		}
	case result := <-p.ch:
		return result
	}
}

func Await[T any](ctx context.Context, task Task[T]) *Promise[T] {
	p := &Promise[T]{
		ch:  make(chan TaskResult[T]),
		ctx: ctx,
	}
	go p.run(task)
	return p
}
