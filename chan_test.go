package parallel

import (
	"sync"
	"testing"
)

func TestMergeChanWithBuffer(t *testing.T) {
	ch1 := make(chan int, 3)
	ch2 := make(chan int, 3)
	ch3 := make(chan int, 3)

	// Send some data
	for i := 0; i < 3; i++ {
		ch1 <- i
		ch2 <- i + 10
		ch3 <- i + 20
	}
	close(ch1)
	close(ch2)
	close(ch3)

	merged := MergeChanWithBuffer(10, ch1, ch2, ch3)

	// Collect results
	var results []int
	for v := range merged {
		results = append(results, v)
	}

	if len(results) != 9 {
		t.Errorf("expected 9 results, got %d", len(results))
	}
}

func TestMergeChanWithBuffer_NilChannels(t *testing.T) {
	ch1 := make(chan int, 2)
	ch1 <- 1
	ch1 <- 2
	close(ch1)

	// Include a nil channel
	merged := MergeChanWithBuffer(5, ch1, nil)

	var results []int
	for v := range merged {
		results = append(results, v)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestMergeChanWithBuffer_NoChannels(t *testing.T) {
	merged := MergeChanWithBuffer[string](5)

	// Should be closed immediately
	_, ok := <-merged
	if ok {
		t.Error("expected closed channel, but got open")
	}
}

func TestMergeChanWithBuffer_ParallelSend(t *testing.T) {
	ch1 := make(chan int)
	ch2 := make(chan int)
	merged := MergeChanWithBuffer(10, ch1, ch2)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			ch1 <- i
		}
		close(ch1)
	}()
	go func() {
		defer wg.Done()
		for i := 10; i < 20; i++ {
			ch2 <- i
		}
		close(ch2)
	}()

	count := 0
	for range merged {
		count++
	}
	wg.Wait()
	if count != 20 {
		t.Errorf("expected 20 results, got %d", count)
	}
}
