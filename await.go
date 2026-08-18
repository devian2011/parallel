package parallel

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
)

// Promise represents an asynchronous task. It holds the eventual result (or error)
// of the task, including recovery from panics. Get() blocks until the task completes
// or the context is canceled. It can be called multiple times safely; subsequent calls
// return the cached result without blocking.
//
// The zero value of Promise is not usable; always create a Promise with Await().
type Promise[T any] struct {
	ctx    context.Context // parent context (stored for informational purposes)
	result T
	err    error
	wait   chan struct{} // closed when the task finishes or context is canceled
}

// run executes the task in a new goroutine, recovers panics, and stores the result
// in the Promise. It is called internally by Await(). This function handles context
// cancellation: if ctx is canceled before the task finishes, it immediately closes
// the wait channel and stores ctx.Err(), unblocking any pending Get() calls.
// The task is still running in the background, but its result is discarded.
// To actually stop the task, the task itself must monitor ctx.Done().
func (p *Promise[T]) run(ctx context.Context, task func(ctx context.Context) (T, error)) {
	// Always close p.wait when we exit, unblocking all Get() calls.
	defer close(p.wait)

	// Channel that signals task completion (including panic).
	done := make(chan struct{})

	// Local variables to avoid data races: the main goroutine writes to p.result/p.err
	// only after receiving from done or ctx.Done(). The task goroutine writes to these locals.
	var result T
	var taskErr error

	// Spawn the actual task in a separate goroutine.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// On panic: return zero value and a wrapped error with stack trace.
				result = *new(T)
				taskErr = errors.Join(ErrFnPanic,
					fmt.Errorf("panic: %v\n%s", r, string(debug.Stack())))
			}
			close(done) // signal that the task has finished (or panicked)
		}()
		// Execute the user-provided task. It must respect ctx cancellation itself.
		result, taskErr = task(ctx)
	}()

	// Wait for either the task to finish or the context to be canceled.
	select {
	case <-done:
		// Task finished – store its result.
		p.result = result
		p.err = taskErr
	case <-ctx.Done():
		// Context canceled – we don't wait for the task.
		// Return zero value and the context error.
		p.result = *new(T)
		p.err = ctx.Err()
		// Note: the task goroutine may still be running; its result is ignored.
	}
}

// Get waits for the background task to complete and returns its result.
// If the context was canceled before the task finished, Get returns ctx.Err()
// immediately (after the Promise processes the cancellation). If the task panicked,
// the panic is recovered and returned as an error (wrapping ErrFnPanic).
//
// Get can be called multiple times; subsequent calls return the cached result
// without blocking because the wait channel is closed.
func (p *Promise[T]) Get() (T, error) {
	<-p.wait // blocks until the channel is closed (task done or context canceled)
	return p.result, p.err
}

// Await starts the given task asynchronously and returns a Promise that can be
// used to retrieve the result. The task runs in its own goroutine.
//
// The provided context is passed to the task and is also used by the Promise itself
// to detect cancellation. If the context is canceled before the task finishes,
// Get() will return ctx.Err() without waiting for the task (the task continues to
// run but its result is ignored). To cancel the task's internal work, the task
// function must check ctx.Done() and return early.
//
// If the task panics, the panic is recovered and returned as an error via Get().
// The returned error will wrap ErrFnPanic and include the stack trace.
//
// The Promise can be used from multiple goroutines safely.
func Await[T any](ctx context.Context, task func(ctx context.Context) (T, error)) *Promise[T] {
	p := &Promise[T]{
		ctx:  ctx,
		wait: make(chan struct{}), // unbuffered; close will unblock all readers
	}
	go p.run(ctx, task)
	return p
}
