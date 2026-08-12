package parallel

import "context"

type TaskResult[T any] struct {
	Value T
	Err   error
}

type Task[T any] func(ctx context.Context) TaskResult[T]
