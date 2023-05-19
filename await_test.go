package parallel

import (
	"context"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestAwait(t *testing.T) {
	const (
		ContextStringResult = "ctx string result"
	)
	promise := Await(context.Background(), func() AwaitTaskResult {
		return &AwaitTaskResultImpl{
			value: ContextStringResult,
			err:   nil,
		}
	})

	ctxTimeout, _ := context.WithTimeout(context.Background(), 1*time.Millisecond)
	promiseTm := Await(ctxTimeout, func() AwaitTaskResult {
		time.Sleep(1 * time.Second)
		return &AwaitTaskResultImpl{
			value: nil,
			err:   nil,
		}
	})

	assert.Equal(t, ContextStringResult, promise.Get().GetValue().(string))
	assert.Equal(t, ErrAwaitBreaks, promiseTm.Get().GetError())
}
