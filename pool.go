package parallel

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

// Predefined errors returned by the pool.
var (
	// TaskPanicErr is returned when a task panics during execution.
	TaskPanicErr = errors.New("task panic")

	// TaskContextErr is returned when a task is submitted with a context
	// that is already cancelled. The task is not executed.
	TaskContextErr = errors.New("task context error")

	// TaskBusyErr is returned when the pool is full and the BusyInputStrategy
	// is set to SendErrOnBusy. The task is rejected without execution.
	TaskBusyErr = errors.New("task rejected: pool is full")

	// PoolCfgValidationErr wraps configuration validation errors.
	PoolCfgValidationErr = errors.New("pool config invalid")

	// PoolIsNotRunningErr is returned when a task is submitted to a pool
	// that is not in the Running state.
	PoolIsNotRunningErr = errors.New("pool is not running")
)

// PoolStatus represents the current lifecycle state of the pool.
type PoolStatus string

const (
	PoolStatusCreated   PoolStatus = "created"
	PoolStatusRunning   PoolStatus = "running"
	PoolStatusSuspended PoolStatus = "suspended"
	PoolStatusStopped   PoolStatus = "stopped"
)

// BusyInputStrategy defines behavior when all workers are busy.
type BusyInputStrategy uint8

const (
	// WaitOnBusy blocks until a worker becomes available or the pool is shut down.
	WaitOnBusy BusyInputStrategy = iota
	// SendErrOnBusy returns TaskBusyErr immediately.
	SendErrOnBusy
	// SilentSkipOnBusy silently drops the task and returns nil.
	SilentSkipOnBusy
)

// OutputStrategy defines behavior for delivering task results.
type OutputStrategy uint8

const (
	// WaitOutput blocks until the result is delivered.
	WaitOutput OutputStrategy = iota
	// SkipOutput drops the result if the output channel is full.
	SkipOutput
)

// PoolCfg holds the configuration for the worker pool.
// It is safe to modify at runtime via UpdateCfg.
type PoolCfg struct {
	busyInputStrategy       BusyInputStrategy
	outStrategy             OutputStrategy
	minWorkers              int32
	maxWorkers              int32
	outputChannelBufferSize uint
	idleTimeout             time.Duration
}

// NewPoolConfig creates a new PoolCfg with the given parameters.
func NewPoolConfig(
	busyInputStrategy BusyInputStrategy,
	outStrategy OutputStrategy,
	minWorkers, maxWorkers int32,
	outputChannelBufferSize uint,
	idleTimeout time.Duration,
) *PoolCfg {
	return &PoolCfg{
		busyInputStrategy:       busyInputStrategy,
		outStrategy:             outStrategy,
		minWorkers:              minWorkers,
		maxWorkers:              maxWorkers,
		outputChannelBufferSize: outputChannelBufferSize,
		idleTimeout:             idleTimeout,
	}
}

// validate checks that the configuration is internally consistent.
func (cfg *PoolCfg) validate() error {
	if cfg.minWorkers < 0 {
		return errors.New("minWorkers must not be negative")
	}
	if cfg.maxWorkers < cfg.minWorkers {
		return errors.New("maxWorkers must be >= minWorkers")
	}
	return nil
}

// PoolWorkerFn is the function type executed by the pool workers.
type PoolWorkerFn func(ctx context.Context) PoolTaskResult

// PoolTask represents a unit of work submitted to the pool.
type PoolTask struct {
	ctx context.Context
	fn  PoolWorkerFn
}

// PoolTaskResult is the interface for task results.
type PoolTaskResult interface {
	GetResult() any
	GetError() error
}

// PoolTaskResultImpl is the default implementation of PoolTaskResult.
type PoolTaskResultImpl struct {
	Result any
	Err    error
}

func (p *PoolTaskResultImpl) GetResult() any { return p.Result }
func (p *PoolTaskResultImpl) GetError() error { return p.Err }

// Pool manages a dynamic worker pool with configurable concurrency and strategies.
// It is safe for concurrent use from multiple goroutines.
type Pool struct {
	cfg atomic.Pointer[PoolCfg] // atomically updated configuration

	wg sync.WaitGroup // tracks all workers

	ctxMtx   sync.RWMutex // protects wCtx and stopFn
	wCtx     context.Context
	stopFn   context.CancelFunc

	statusMtx sync.RWMutex
	status    PoolStatus

	tasksMtx sync.Mutex
	tasks    chan PoolTask
	output   chan PoolTaskResult

	activeWorkers atomic.Int32
}

// NewPool creates a new Pool instance with the given configuration.
// The pool starts in the Created state. Call Start() to begin processing.
func NewPool(cfg *PoolCfg) (*Pool, error) {
	if err := cfg.validate(); err != nil {
		return nil, errors.Join(PoolCfgValidationErr, err)
	}
	p := &Pool{
		tasks:  make(chan PoolTask),
		output: make(chan PoolTaskResult, cfg.outputChannelBufferSize),
		status: PoolStatusCreated,
	}
	p.cfg.Store(cfg)
	return p, nil
}

// UpdateCfg replaces the pool configuration at runtime.
// The new configuration is validated before being applied.
func (p *Pool) UpdateCfg(cfg *PoolCfg) error {
	if err := cfg.validate(); err != nil {
		return errors.Join(PoolCfgValidationErr, err)
	}
	p.cfg.Store(cfg)
	return nil
}

// Start transitions the pool to the Running state and spawns the minimum workers.
func (p *Pool) Start() {
	p.statusMtx.Lock()
	defer p.statusMtx.Unlock()

	if p.status == PoolStatusRunning {
		return
	}
	if p.status == PoolStatusStopped {
		cfg := p.cfg.Load()
		p.tasks = make(chan PoolTask)
		p.output = make(chan PoolTaskResult, cfg.outputChannelBufferSize)
		p.activeWorkers.Store(0)
	}

	cfg := p.cfg.Load()
	ctx, cancel := context.WithCancel(context.Background())
	p.ctxMtx.Lock()
	p.wCtx = ctx
	p.stopFn = cancel
	p.ctxMtx.Unlock()

	waitStart := &sync.WaitGroup{}
	waitStart.Add(int(cfg.minWorkers))
	p.wg.Add(int(cfg.minWorkers))
	for i := 0; i < int(cfg.minWorkers); i++ {
		go p.runWorker(waitStart)
	}
	waitStart.Wait()

	p.status = PoolStatusRunning
}

// Suspend puts the pool into the Suspended state. New submissions are rejected.
func (p *Pool) Suspend() {
	p.statusMtx.Lock()
	defer p.statusMtx.Unlock()
	p.status = PoolStatusSuspended
}

// Stop permanently shuts down the pool. After Stop, the pool can be restarted.
func (p *Pool) Stop() {
	p.statusMtx.Lock()
	if p.status == PoolStatusStopped {
		p.statusMtx.Unlock()
		return
	}
	p.status = PoolStatusStopped
	p.statusMtx.Unlock()

	p.ctxMtx.RLock()
	stopFn := p.stopFn
	p.ctxMtx.RUnlock()
	if stopFn != nil {
		stopFn()
	}
	p.wg.Wait()

	p.tasksMtx.Lock()
	close(p.tasks)
	p.tasksMtx.Unlock()
	close(p.output)
}

// Status returns the current lifecycle state of the pool.
func (p *Pool) Status() PoolStatus {
	p.statusMtx.RLock()
	defer p.statusMtx.RUnlock()
	return p.status
}

// GetOutput returns the output channel for task results.
func (p *Pool) GetOutput() <-chan PoolTaskResult {
	return p.output
}

// ActiveWorkers returns the current number of running worker goroutines.
func (p *Pool) ActiveWorkers() int32 {
	return p.activeWorkers.Load()
}

// Submit submits a task to the pool for asynchronous execution.
func (p *Pool) Submit(task PoolTask) error {
	// First attempt: non-blocking send with lock.
	p.tasksMtx.Lock()
	if p.status != PoolStatusRunning {
		p.tasksMtx.Unlock()
		return PoolIsNotRunningErr
	}
	select {
	case p.tasks <- task:
		p.tasksMtx.Unlock()
		return nil
	default:
	}
	p.tasksMtx.Unlock()

	// Try to scale up if possible.
	cfg := p.cfg.Load()
	if p.activeWorkers.Load() < cfg.maxWorkers {
		waitStart := &sync.WaitGroup{}
		waitStart.Add(1)
		p.wg.Add(1)
		go p.runWorker(waitStart)
		waitStart.Wait()
	}

	// Second attempt: non-blocking send (status may have changed).
	p.tasksMtx.Lock()
	if p.status != PoolStatusRunning {
		p.tasksMtx.Unlock()
		return PoolIsNotRunningErr
	}
	select {
	case p.tasks <- task:
		p.tasksMtx.Unlock()
		return nil
	default:
	}
	p.tasksMtx.Unlock()

	// Apply busy strategy based on configuration.
	cfg = p.cfg.Load()
	switch cfg.busyInputStrategy {
	case WaitOnBusy:
		p.ctxMtx.RLock()
		wCtx := p.wCtx
		p.ctxMtx.RUnlock()
		select {
		case p.tasks <- task:
			return nil
		case <-wCtx.Done():
			return PoolIsNotRunningErr
		}
	case SendErrOnBusy:
		return TaskBusyErr
	case SilentSkipOnBusy:
		return nil
	default:
		return nil
	}
}

// runWorker is the main loop for each worker goroutine.
func (p *Pool) runWorker(waitStart *sync.WaitGroup) {
	p.activeWorkers.Add(1)
	defer p.activeWorkers.Add(-1)
	defer p.wg.Done()

	cfg := p.cfg.Load()
	timer := time.NewTimer(cfg.idleTimeout)
	defer timer.Stop()

	waitStart.Done()

	for {
		// Reset idle timer
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		cfg = p.cfg.Load() // refresh config for each loop
		timer.Reset(cfg.idleTimeout)

		p.ctxMtx.RLock()
		wCtx := p.wCtx
		p.ctxMtx.RUnlock()

		select {
		case <-wCtx.Done():
			return

		case <-timer.C:
			// Check if we can scale down
			if p.activeWorkers.Load() > cfg.minWorkers {
				return
			}
			// Otherwise, continue waiting

		case task, ok := <-p.tasks:
			if !ok {
				return // tasks channel closed
			}

			// Check if task context is already cancelled
			if task.ctx.Err() != nil {
				result := &PoolTaskResultImpl{Err: TaskContextErr}
				select {
				case p.output <- result:
				default:
				}
				continue
			}

			// Execute task with panic recovery
			taskResult := func() (result PoolTaskResult) {
				defer func() {
					if r := recover(); r != nil {
						result = &PoolTaskResultImpl{
							Err: errors.Join(
								TaskPanicErr,
								fmt.Errorf("task panic: %v, stack: %s", r, string(debug.Stack())),
							),
						}
					}
				}()
				result = task.fn(task.ctx)
				return result
			}()

			// Deliver result according to output strategy
			cfg = p.cfg.Load() // refresh config before sending
			if cfg.outStrategy == WaitOutput {
				select {
				case p.output <- taskResult:
				case <-wCtx.Done():
					return
				}
			} else { // SkipOutput
				select {
				case p.output <- taskResult:
				default:
				}
			}
		}
	}
}
