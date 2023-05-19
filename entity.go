package parallel

type TaskResult interface {
	GetValue() any
	HasError() bool
	GetError() error
}

type TaskResultImpl struct {
	value any
	err   error
}

func (a *TaskResultImpl) GetValue() any {
	return a.value
}

func (a *TaskResultImpl) HasError() bool {
	return a.err == nil
}

func (a *TaskResultImpl) GetError() error {
	return a.err
}

type Task func() TaskResult
