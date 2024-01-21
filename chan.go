package parallel

import "sync"

func MergeChan[T any](in ...chan T) chan T {
	return MergeChanWithBuffer[T](1, in...)
}

func MergeChanWithBuffer[T any](buffer uint, in ...chan T) chan T {
	out := make(chan T, buffer)
	wg := &sync.WaitGroup{}
	wg.Add(len(in))
	for _, ch := range in {
		ch := ch
		go func(wg *sync.WaitGroup, in chan T, out chan T) {
			for msg := range in {
				out <- msg
			}
			wg.Done()
		}(wg, ch, out)
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}
