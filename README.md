# ⚡ parallel – Concurrent Utilities for Go

Go 1.20+ v1.0.0 [github.com/devian2011/parallel](https://github.com/devian2011/parallel)

Package **parallel** provides a set of production‑ready generic utilities for concurrent programming in Go. It includes a **Promise** (async/await) pattern, parallel processing of channels and slices with configurable worker pools, a channel merger, and a **dynamic worker pool** with auto‑scaling, backpressure, and graceful lifecycle management. All components are thoroughly tested with `-race` and have zero external dependencies.

## 📦 Installation

```
go get github.com/devian2011/parallel
```

## 🔄 Core Components

### 1\. Promise – Async/Await for Go

The `Promise[T]` type represents an asynchronous task. It holds the eventual result (or error) and recovers from panics. Use `Await()` to start a task, and `Get()` to block until completion or context cancellation.

```go
import "github.com/devian2011/parallel"

ctx := context.Background()
promise := parallel.Await(ctx, func(ctx context.Context) (string, error) {
    // Simulate work
    time.Sleep(100 * time.Millisecond)
    return "hello", nil
})

result, err := promise.Get() // blocks
fmt.Println(result) // hello
```

**Features:**

*   Panic recovery – panics are captured and returned as an error wrapping `ErrFnPanic` with stack trace.
*   Context cancellation – if the context is cancelled, `Get()` returns `ctx.Err()` immediately without waiting for the task.
*   Safe for multiple goroutines – `Get()` can be called concurrently; it returns the cached result.

### 2\. Parallel Channel & Slice Processors

Three generic functions for parallel processing of streams and slices:

*   **`HandleParallelChan`** – reads from an input channel, processes each item with a worker pool, and returns a buffered output channel of `TaskResult[OUT]`.
*   **`HandleParallelOut`** – similar to the above, but sends each result to a handler callback (e.g., for side effects like saving to DB). Returns a `WaitGroup` that signals when all processing and handling are done.
*   **`HandleParallelArr`** – processes a slice concurrently, preserving the original order. Supports context cancellation and returns partial results.

**Example – channel processing:**

```go
in := make(chan int, 10)
for i := 0; i < 10; i++ { in <- i }
close(in)

out := parallel.HandleParallelChan(ctx, 4, in, func(n int) parallel.TaskResult[string] {
    return parallel.TaskResult[string]{Value: fmt.Sprintf("n=%d", n)}
})

for res := range out {
    fmt.Println(res.Value)
}
```

**Example – slice processing (order preserved):**

```go
input := []int{1,2,3,4,5}
results, err := parallel.HandleParallelArr(ctx, 3, input, func(n int) parallel.TaskResult[int] {
    return parallel.TaskResult[int]{Value: n * n}
})
for i, r := range results {
    fmt.Printf("[%d] = %d\n", i, r.Value)
}
```

### 3\. MergeChanWithBuffer

Merges multiple channels of the same type into a single output channel. The output channel is buffered and is closed when all input channels are closed and drained. Nil channels are skipped.

```go
ch1 := make(chan int)
ch2 := make(chan int)
// fill and close channels...

merged := parallel.MergeChanWithBuffer(100, ch1, ch2)
for v := range merged {
    fmt.Println(v)
}
```

### 4\. Dynamic Worker Pool

The package includes a full‑featured worker pool with auto‑scaling, configurable backpressure, and graceful shutdown. It is designed for high‑throughput task submission and result collection.

#### Configuration

The pool is configured via `PoolCfg` created with `NewPoolConfig`:

```go
cfg := parallel.NewPoolConfig(
    parallel.WaitOnBusy,      // busy strategy
    parallel.WaitOutput,      // output strategy
    2,                        // min workers (always alive)
    10,                       // max workers (scale up limit)
    100,                      // output channel buffer size
    5 * time.Second,          // idle timeout before scale down
)
```

#### Lifecycle

```go
pool, err := parallel.NewPool(cfg)
if err != nil {
    log.Fatal(err)
}
pool.Start()
defer pool.Stop() // graceful shutdown
```

#### Submit a task

```go
task := parallel.PoolTask{
    ctx: context.Background(),
    fn: func(ctx context.Context) parallel.PoolTaskResult {
        // do some work
        return ¶llel.PoolTaskResultImpl{Result: "success"}
    },
}
err := pool.Submit(task)
if err != nil {
    // handle error (PoolIsNotRunningErr, TaskBusyErr, etc.)
}
```

#### Read results

```go
for result := range pool.GetOutput() {
    fmt.Println(result.GetResult())
    if err := result.GetError(); err != nil {
        // task failed
    }
}
```

#### Suspend / Resume

```go
pool.Suspend()   // reject new submissions, active tasks continue
// ... later
pool.Start()     // resume accepting tasks
```

#### Update config at runtime

```go
newCfg := parallel.NewPoolConfig(
    parallel.SendErrOnBusy, // change busy strategy
    parallel.SkipOutput,    // change output strategy
    2, 20, 200, 3*time.Second,
)
_ = pool.UpdateCfg(newCfg)
```

#### Busy strategies (when all workers are busy)

| Strategy | Behaviour |
| --- | --- |
| `WaitOnBusy` | `Submit()` blocks until a worker becomes available or pool stops |
| `SendErrOnBusy` | `Submit()` returns `TaskBusyErr` immediately |
| `SilentSkipOnBusy` | `Submit()` returns `nil` (task silently dropped) |

#### Output strategies (result delivery)

| Strategy | Behaviour |
| --- | --- |
| `WaitOutput` | Worker blocks until result is written to output channel |
| `SkipOutput` | Non‑blocking send; result is dropped if output channel is full |

#### Errors returned by Pool

*   `TaskPanicErr` – task panicked (stack trace included)
*   `TaskContextErr` – task context already cancelled
*   `TaskBusyErr` – pool full and strategy `SendErrOnBusy`
*   `PoolCfgValidationErr` – invalid configuration
*   `PoolIsNotRunningErr` – pool not in `Running` state

#### Auto‑scaling

The pool dynamically adjusts the number of workers between `minWorkers` and `maxWorkers`. When tasks arrive and all existing workers are busy, it scales up (up to `maxWorkers`). If a worker remains idle for longer than `idleTimeout` and the current worker count is above `minWorkers`, it shuts down gracefully.

#### Full example

```go
package main

import (
    "context"
    "fmt"
    "time"
    "github.com/devian2011/parallel"
)

func main() {
    cfg := parallel.NewPoolConfig(
        parallel.WaitOnBusy,
        parallel.WaitOutput,
        2, 8, 50, 2*time.Second,
    )
    pool, _ := parallel.NewPool(cfg)
    pool.Start()
    defer pool.Stop()

    // Submit 20 tasks
    for i := 0; i < 20; i++ {
        n := i
        task := parallel.PoolTask{
            ctx: context.Background(),
            fn: func(ctx context.Context) parallel.PoolTaskResult {
                time.Sleep(20 * time.Millisecond)
                return ¶llel.PoolTaskResultImpl{Result: n * n}
            },
        }
        _ = pool.Submit(task)
    }

    // Collect results
    count := 0
    for res := range pool.GetOutput() {
        fmt.Printf("result: %v\n", res.GetResult())
        count++
        if count == 20 {
            break
        }
    }
}
```

### 5\. Generic Types & Errors

*   `TaskResult[T]` – holds a value of type T and an optional error.
*   `Task[T]` – a function type `func(ctx context.Context) TaskResult[T]`.
*   `ErrFnPanic` – sentinel error returned when a task panics; it wraps the panic message and stack trace.

## 📌 Best Practices

*   **Choose the right busy strategy** – `WaitOnBusy` provides backpressure, `SendErrOnBusy` gives immediate feedback, `SilentSkipOnBusy` for non‑critical tasks.
*   **Match output strategy to consumer speed** – `WaitOutput` ensures no loss but may block workers; `SkipOutput` prevents blocking but may drop results.
*   **Set idle timeout** – prevents worker bloat during low load.
*   **Use context cancellation** – tasks should check `ctx.Done()` to support graceful shutdown.
*   **Recover panics** – the pool and Promise already recover and return errors; no need for extra `recover` in your tasks.
*   **For slice processing** – use `HandleParallelArr` when order matters; it's built for that.
*   **For streaming data** – use `HandleParallelChan` or `HandleParallelOut`.

## 🧪 Testing

The package includes comprehensive tests with race detection. Run:

```
go test -race -v ./...
```

## 📄 License

This project is licensed under the **GNU General Public License v3.0**. See the [LICENSE](https://www.gnu.org/licenses/gpl-3.0.en.html) file for details.
