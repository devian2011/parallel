package parallel

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestPool_Basic(t *testing.T) {
	cfg := NewPoolConfig(WaitOnBusy, WaitOutput, 1, 2, 10, 100*time.Millisecond)
	p, err := NewPool(cfg)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}
	p.Start()
	defer p.Stop()

	ctx := context.Background()
	fn := func(ctx context.Context) PoolTaskResult {
		return &PoolTaskResultImpl{Result: 42}
	}
	err = p.Submit(PoolTask{ctx: ctx, fn: fn})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	select {
	case res := <-p.output:
		if res.GetResult() != 42 {
			t.Errorf("expected 42, got %v", res.GetResult())
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for result")
	}
}

func TestPool_TaskContextCancel(t *testing.T) {
	cfg := NewPoolConfig(WaitOnBusy, WaitOutput, 1, 1, 10, time.Second)
	p, err := NewPool(cfg)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}
	p.Start()
	defer p.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	fn := func(ctx context.Context) PoolTaskResult {
		return &PoolTaskResultImpl{Result: "should not run"}
	}
	err = p.Submit(PoolTask{ctx: ctx, fn: fn})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	select {
	case res := <-p.output:
		if !errors.Is(res.GetError(), TaskContextErr) {
			t.Errorf("expected TaskContextErr, got %v", res.GetError())
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for result")
	}
}

func TestPool_PanicRecovery(t *testing.T) {
	cfg := NewPoolConfig(WaitOnBusy, WaitOutput, 1, 1, 10, time.Second)
	p, err := NewPool(cfg)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}
	p.Start()
	defer p.Stop()

	ctx := context.Background()
	fn := func(ctx context.Context) PoolTaskResult {
		panic("oops")
	}
	err = p.Submit(PoolTask{ctx: ctx, fn: fn})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	select {
	case res := <-p.output:
		if !errors.Is(res.GetError(), TaskPanicErr) {
			t.Errorf("expected TaskPanicErr, got %v", res.GetError())
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for result")
	}
}

func TestPool_BusyStrategySendErr(t *testing.T) {
	cfg := NewPoolConfig(SendErrOnBusy, SkipOutput, 1, 1, 10, time.Second)
	p, err := NewPool(cfg)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}
	p.Start()
	defer p.Stop()

	// Fill the worker with a long-running task.
	ctx := context.Background()
	longTask := func(ctx context.Context) PoolTaskResult {
		time.Sleep(200 * time.Millisecond)
		return &PoolTaskResultImpl{Result: "done"}
	}
	// Submit first task (will occupy worker)
	err = p.Submit(PoolTask{ctx: ctx, fn: longTask})
	if err != nil {
		t.Fatalf("Submit first task failed: %v", err)
	}

	// Second task should be rejected immediately.
	shortTask := func(ctx context.Context) PoolTaskResult {
		return &PoolTaskResultImpl{Result: "should not run"}
	}
	err = p.Submit(PoolTask{ctx: ctx, fn: shortTask})
	if !errors.Is(err, TaskBusyErr) {
		t.Errorf("expected TaskBusyErr, got %v", err)
	}
}

func TestPool_BusyStrategySilentSkip(t *testing.T) {
	cfg := NewPoolConfig(SilentSkipOnBusy, SkipOutput, 1, 1, 10, time.Second)
	p, err := NewPool(cfg)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}
	p.Start()
	defer p.Stop()

	// Fill the worker.
	ctx := context.Background()
	longTask := func(ctx context.Context) PoolTaskResult {
		time.Sleep(200 * time.Millisecond)
		return &PoolTaskResultImpl{Result: "done"}
	}
	err = p.Submit(PoolTask{ctx: ctx, fn: longTask})
	if err != nil {
		t.Fatalf("Submit first task failed: %v", err)
	}

	// Second task should be silently skipped (no error).
	err = p.Submit(PoolTask{ctx: ctx, fn: longTask})
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestPool_WaitOnBusy(t *testing.T) {
	cfg := NewPoolConfig(WaitOnBusy, WaitOutput, 1, 1, 10, time.Second)
	p, err := NewPool(cfg)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}
	p.Start()
	defer p.Stop()

	// Fill the worker.
	ctx := context.Background()
	longTask := func(ctx context.Context) PoolTaskResult {
		time.Sleep(200 * time.Millisecond)
		return &PoolTaskResultImpl{Result: "done"}
	}
	err = p.Submit(PoolTask{ctx: ctx, fn: longTask})
	if err != nil {
		t.Fatalf("Submit first task failed: %v", err)
	}

	// Second task should block until worker is free.
	start := time.Now()
	err = p.Submit(PoolTask{ctx: ctx, fn: longTask})
	elapsed := time.Since(start)
	if err != nil {
		t.Errorf("Submit second task failed: %v", err)
	}
	if elapsed < 150*time.Millisecond {
		t.Errorf("expected blocking for ~200ms, got %v", elapsed)
	}
}

func TestPool_OutputStrategySkip(t *testing.T) {
	cfg := NewPoolConfig(WaitOnBusy, SkipOutput, 1, 2, 1, time.Second) // buffer size 1
	p, err := NewPool(cfg)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}
	p.Start()
	defer p.Stop()

	ctx := context.Background()
	fn := func(ctx context.Context) PoolTaskResult {
		return &PoolTaskResultImpl{Result: "result"}
	}

	// Submit 3 tasks, output channel capacity is 1, but SkipOutput will drop overflow.
	for i := 0; i < 3; i++ {
		err = p.Submit(PoolTask{ctx: ctx, fn: fn})
		if err != nil {
			t.Fatalf("Submit failed: %v", err)
		}
	}

	// Wait a bit for workers to finish.
	time.Sleep(100 * time.Millisecond)

	// Count how many results we can read.
	count := 0
	for {
		select {
		case <-p.output:
			count++
		default:
			goto done
		}
	}
done:
	if count == 0 {
		t.Error("expected at least one result, got 0")
	}
	if count > 3 {
		t.Errorf("expected at most 3 results, got %d", count)
	}
}

func TestPool_Scaling(t *testing.T) {
	cfg := NewPoolConfig(WaitOnBusy, WaitOutput, 1, 3, 10, time.Second)
	p, err := NewPool(cfg)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}
	p.Start()
	defer p.Stop()

	ctx := context.Background()
	fn := func(ctx context.Context) PoolTaskResult {
		time.Sleep(50 * time.Millisecond)
		return &PoolTaskResultImpl{Result: 42}
	}

	// Submit 5 tasks; pool should scale up to maxWorkers (3).
	for i := 0; i < 5; i++ {
		err = p.Submit(PoolTask{ctx: ctx, fn: fn})
		if err != nil {
			t.Fatalf("Submit failed: %v", err)
		}
	}

	// Wait for all tasks to complete.
	for i := 0; i < 5; i++ {
		<-p.output
	}

	// Active workers should have scaled up to 3 (or more, but max is 3).
	workers := p.ActiveWorkers()
	if workers != 3 {
		t.Errorf("expected 3 active workers, got %d", workers)
	}
}

func TestPool_IdleTimeout(t *testing.T) {
	cfg := NewPoolConfig(WaitOnBusy, WaitOutput, 1, 3, 10, 100*time.Millisecond)
	p, err := NewPool(cfg)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}
	p.Start()
	defer p.Stop()

	ctx := context.Background()
	fn := func(ctx context.Context) PoolTaskResult {
		return &PoolTaskResultImpl{Result: 42}
	}

	// Submit tasks to scale up to 3 workers.
	for i := 0; i < 3; i++ {
		err = p.Submit(PoolTask{ctx: ctx, fn: fn})
		if err != nil {
			t.Fatalf("Submit failed: %v", err)
		}
	}
	// Wait for tasks to finish.
	for i := 0; i < 3; i++ {
		<-p.output
	}

	// Wait for idle timeout to scale down.
	time.Sleep(200 * time.Millisecond)

	workers := p.ActiveWorkers()
	if workers != 1 {
		t.Errorf("expected 1 active worker after idle timeout, got %d", workers)
	}
}

func TestPool_StopAndRestart(t *testing.T) {
	cfg := NewPoolConfig(WaitOnBusy, WaitOutput, 1, 2, 10, time.Second)
	p, err := NewPool(cfg)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}
	p.Start()

	ctx := context.Background()
	fn := func(ctx context.Context) PoolTaskResult {
		return &PoolTaskResultImpl{Result: "first run"}
	}
	err = p.Submit(PoolTask{ctx: ctx, fn: fn})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	<-p.output

	p.Stop()
	if p.Status() != PoolStatusStopped {
		t.Errorf("expected Stopped, got %v", p.Status())
	}

	// Restart
	p.Start()
	if p.Status() != PoolStatusRunning {
		t.Errorf("expected Running after restart, got %v", p.Status())
	}

	// Submit another task.
	fn2 := func(ctx context.Context) PoolTaskResult {
		return &PoolTaskResultImpl{Result: "second run"}
	}
	err = p.Submit(PoolTask{ctx: ctx, fn: fn2})
	if err != nil {
		t.Fatalf("Submit after restart failed: %v", err)
	}
	select {
	case res := <-p.output:
		if res.GetResult() != "second run" {
			t.Errorf("expected 'second run', got %v", res.GetResult())
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for result after restart")
	}
	p.Stop()
}

func TestPool_Suspend(t *testing.T) {
	cfg := NewPoolConfig(WaitOnBusy, WaitOutput, 1, 2, 10, time.Second)
	p, err := NewPool(cfg)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}
	p.Start()
	defer p.Stop()

	// Submit a long task.
	ctx := context.Background()
	longTask := func(ctx context.Context) PoolTaskResult {
		time.Sleep(200 * time.Millisecond)
		return &PoolTaskResultImpl{Result: "done"}
	}
	err = p.Submit(PoolTask{ctx: ctx, fn: longTask})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	// Suspend the pool.
	p.Suspend()
	if p.Status() != PoolStatusSuspended {
		t.Errorf("expected Suspended, got %v", p.Status())
	}

	// Try to submit another task; should fail.
	err = p.Submit(PoolTask{ctx: ctx, fn: longTask})
	if !errors.Is(err, PoolIsNotRunningErr) {
		t.Errorf("expected PoolIsNotRunningErr, got %v", err)
	}

	// The first task should still complete.
	select {
	case <-p.output:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for result of active task")
	}

	// Resume.
	p.Start()
	if p.Status() != PoolStatusRunning {
		t.Errorf("expected Running after resume, got %v", p.Status())
	}
}

func TestPool_UpdateConfig(t *testing.T) {
	cfg := NewPoolConfig(WaitOnBusy, WaitOutput, 1, 2, 10, time.Second)
	p, err := NewPool(cfg)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}
	p.Start()
	defer p.Stop()

	// Update maxWorkers to 5.
	newCfg := NewPoolConfig(WaitOnBusy, WaitOutput, 1, 5, 10, time.Second)
	err = p.UpdateCfg(newCfg)
	if err != nil {
		t.Fatalf("UpdateCfg failed: %v", err)
	}

	// Submit tasks to trigger scaling.
	ctx := context.Background()
	fn := func(ctx context.Context) PoolTaskResult {
		time.Sleep(50 * time.Millisecond)
		return &PoolTaskResultImpl{Result: 42}
	}
	for i := 0; i < 5; i++ {
		err = p.Submit(PoolTask{ctx: ctx, fn: fn})
		if err != nil {
			t.Fatalf("Submit failed: %v", err)
		}
	}
	// Wait for completion.
	for i := 0; i < 5; i++ {
		<-p.output
	}
	// Active workers should be 5 (max).
	workers := p.ActiveWorkers()
	if workers != 5 {
		t.Errorf("expected 5 active workers, got %d", workers)
	}
}

func TestPool_ConcurrentSubmit(t *testing.T) {
	cfg := NewPoolConfig(WaitOnBusy, WaitOutput, 2, 4, 20, time.Second)
	p, err := NewPool(cfg)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}
	p.Start()
	defer p.Stop()

	ctx := context.Background()
	fn := func(ctx context.Context) PoolTaskResult {
		time.Sleep(10 * time.Millisecond)
		return &PoolTaskResultImpl{Result: 1}
	}

	const numTasks = 50
	var wg sync.WaitGroup
	wg.Add(numTasks)
	for i := 0; i < numTasks; i++ {
		go func() {
			defer wg.Done()
			err := p.Submit(PoolTask{ctx: ctx, fn: fn})
			if err != nil {
				t.Errorf("Submit failed: %v", err)
			}
		}()
	}

	// Wait for all results.
	count := 0
	for i := 0; i < numTasks; i++ {
		select {
		case <-p.output:
			count++
		case <-time.After(time.Second):
			t.Errorf("timeout waiting for result %d", i)
		}
	}

	// Wait for all submits to finish.
	wg.Wait()

	if count != numTasks {
		t.Errorf("expected %d results, got %d", numTasks, count)
	}
}

func TestPool_SubmitToStoppedPool(t *testing.T) {
	cfg := NewPoolConfig(WaitOnBusy, WaitOutput, 1, 1, 10, time.Second)
	p, err := NewPool(cfg)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}
	p.Start()
	p.Stop() // stop immediately

	ctx := context.Background()
	fn := func(ctx context.Context) PoolTaskResult {
		return &PoolTaskResultImpl{Result: 42}
	}
	err = p.Submit(PoolTask{ctx: ctx, fn: fn})
	if !errors.Is(err, PoolIsNotRunningErr) {
		t.Errorf("expected PoolIsNotRunningErr, got %v", err)
	}
}
