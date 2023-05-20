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
	promise := Await(context.Background(), func() TaskResult {
		return &TaskResultImpl{
			value: ContextStringResult,
			err:   nil,
		}
	})

	ctxTimeout, st := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer st()

	promiseTm := Await(ctxTimeout, func() TaskResult {
		time.Sleep(1 * time.Second)
		return &TaskResultImpl{
			value: nil,
			err:   nil,
		}
	})

	assert.Equal(t, ContextStringResult, promise.Get().GetValue().(string))
	assert.Equal(t, ErrAwaitBreaks, promiseTm.Get().GetError())
}
