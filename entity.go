package parallel

type TaskResult[T any] interface {
	GetValue() T
	HasError() bool
	GetError() error
}

type TaskResultImpl[T any] struct {
	value T
	err   error
}

func (a *TaskResultImpl[T]) GetValue() T {
	return a.value
}

func (a *TaskResultImpl[T]) HasError() bool {
	return a.err != nil
}

func (a *TaskResultImpl[T]) GetError() error {
	return a.err
}

type Task[T any] func() TaskResult[T]
