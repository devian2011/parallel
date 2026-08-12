package parallel

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
)

// ErrAwaitPanic is returned when a task panics during execution.
// It wraps the original panic message and stack trace.
var ErrAwaitPanic = errors.New("await task panic")

// Promise represents a pending asynchronous task. It provides a way to
// obtain the result of a Task executed in a separate goroutine.
// The zero value is not usable; use Await() to create a Promise.
type Promise[T any] struct {
	result TaskResult[T]
	ctx    context.Context // parent context (passed to the task)
	wg     sync.WaitGroup  // signals when the background task has finished
}

// run executes the task in a separate goroutine, captures panics, and stores
// the result in the Promise. It is called internally by Await().
func (p *Promise[T]) run(ctx context.Context, task Task[T]) {
	defer p.wg.Done()
	defer func() {
		if err := recover(); err != nil {
			// Recovered panic: store an error result.
			p.result = TaskResult[T]{
				Err: errors.Join(ErrAwaitPanic, fmt.Errorf("err: %v stack: %s", err, string(debug.Stack()))),
			}
		}
	}()
	// Execute the task and store its result.
	// The task is responsible for checking ctx.Done() and handling cancellation.
	p.result = task(ctx)
}

// Get waits for the background task to complete and returns its result.
// It can be called multiple times safely; subsequent calls return the same
// cached result without blocking.
// Note: the task must respect the provided context and return an appropriate
// error if the context is cancelled. Get() does not override the result
// based on context cancellation; it simply waits for the task to finish.
func (p *Promise[T]) Get() TaskResult[T] {
	p.wg.Wait()
	return p.result
}

// Await starts the given task asynchronously and returns a Promise that
// can be used to obtain the result. The task is executed in a new goroutine.
// The provided context can be used by the task to handle cancellation.
// If the task panics, the panic is recovered and returned as an error
// in the result.
func Await[T any](ctx context.Context, task Task[T]) *Promise[T] {
	p := &Promise[T]{
		ctx: ctx,
	}
	p.wg.Add(1)
	go p.run(ctx, task)
	return p
}
