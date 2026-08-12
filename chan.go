package parallel

import (
	"sync"
)

// MergeChanWithBuffer merges multiple input channels into a single output channel.
// All input channels must be of the same type T. The output channel is buffered with
// the given buffer size. The function returns a channel that will be closed after
// all input channels are closed and drained.
//
// Important:
//   - The caller must ensure that all input channels are eventually closed,
//     otherwise the merge will hang indefinitely.
//   - Input channels should not be nil; if any input channel is nil, it is skipped
//     (the function will not block, but you won't receive any data from it).
//   - If no input channels are provided, the returned channel is closed immediately.
//
// This function is safe for concurrent use.
func MergeChanWithBuffer[T any](buffer uint, in ...chan T) chan T {
	if len(in) == 0 {
		// Return a closed channel to avoid blocking.
		ch := make(chan T, buffer)
		close(ch)
		return ch
	}

	out := make(chan T, buffer)
	wg := &sync.WaitGroup{}
	wg.Add(len(in))

	for _, ch := range in {
		if ch == nil {
			// Skip nil channels to avoid deadlock.
			wg.Done()
			continue
		}
		// Capture loop variable.
		ch := ch
		go func() {
			defer wg.Done()
			for msg := range ch {
				out <- msg
			}
		}()
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}
