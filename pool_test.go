package parallel

import (
	"errors"
	"github.com/stretchr/testify/assert"
	"runtime"
	"testing"
)

func TestPool_Execute(t *testing.T) {
	errMessage := errors.New("errmessage")
	pool := NewPool[string](runtime.NumCPU(), func(in string) (any, error) {
		if in == "err" {
			return nil, errMessage
		}
		return in, nil
	})

	oneResult := pool.Execute("one")
	twoResult := pool.Execute("two")
	errResult := pool.Execute("err")

	assert.Equal(t, "one", oneResult.GetValue().(string))
	assert.Equal(t, "two", twoResult.GetValue().(string))
	assert.Equal(t, errMessage, errResult.GetError())

	pool.Stop()
}
