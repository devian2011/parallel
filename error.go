package parallel

import "errors"

// ErrFnPanic is returned when a task panics during execution.
// It wraps the original panic message and stack trace.
var ErrFnPanic = errors.New("await task panic")
