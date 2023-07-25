package parallel

import (
	"context"
	"errors"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPool_Execute(t *testing.T) {
	errMessage := errors.New("errmessage")
	pool := NewPool[string, string](context.Background(), runtime.NumCPU(), func(in string) (any, error) {
		if in == "err" {
			return nil, errMessage
		}
		return in, nil
	})

	oneResult := pool.Execute("one")
	twoResult := pool.Execute("two")
	errResult := pool.Execute("err")

	assert.Equal(t, "one", oneResult.GetValue())
	assert.Equal(t, "two", twoResult.GetValue())
	assert.Equal(t, errMessage, errResult.GetError())

	pool.Stop()
}
