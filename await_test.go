package parallel

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAwait(t *testing.T) {
	const (
		ContextStringResult = "ctx string result"
	)
	promise := Await[string](context.Background(), func() TaskResult[string] {
		return &TaskResultImpl[string]{
			value: ContextStringResult,
			err:   nil,
		}
	})

	ctxTimeout, st := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer st()

	promiseTm := Await[string](ctxTimeout, func() TaskResult[string] {
		time.Sleep(1 * time.Second)
		return &TaskResultImpl[string]{
			value: "",
			err:   nil,
		}
	})

	assert.Equal(t, ContextStringResult, promise.Get().GetValue())
	assert.Equal(t, ErrAwaitBreaks, promiseTm.Get().GetError())
}
