# parallel Concurrent worker pool & channel utilities for Go

v1.0.0 go get `go get github.com/devian2011/parallel`

Package **parallel** provides a production‑ready dynamic worker pool with auto‑scaling, configurable backpressure, and graceful lifecycle management, plus a set of generic utilities for parallel processing of channels and slices. All components are fully thread‑safe, tested with `-race`, and have zero external dependencies.

**📑 Table of Contents**

*   [🔄 Pool – dynamic worker pool]
*   [🧩 Utilities – channels & slices]
*   [🔀 MergeChanWithBuffer]
*   [🧩 Generic types – TaskResult, Task]
*   [📐 Full examples]
*   [📌 Best practices]

## 🔄 Pool – dynamic worker pool

### Features

*   **Auto‑scaling** – scales up to `maxWorkers` under load, down to `minWorkers` after `idleTimeout`.
*   **3 busy strategies**: `WaitOnBusy`, `SendErrOnBusy`, `SilentSkipOnBusy`.
*   **2 output strategies**: `WaitOutput` (blocking) and `SkipOutput` (non‑blocking, drops if full).
*   **Lifecycle**: `Start`, `Stop` (graceful shutdown), `Suspend`/`Resume`.
*   **Panic recovery** – panics are caught and returned as `TaskPanicErr` with stack trace.
*   **Context cancellation** – tasks can check `ctx.Done()` to abort early.
*   **Atomic config updates** – change configuration at runtime without restarting the pool.

### Configuration

```
cfg := parallel.NewPoolConfig(
    parallel.WaitOnBusy,      // busy strategy
    parallel.WaitOutput,      // output strategy
    2,                        // min workers (always alive)
    10,                       // max workers (scale up limit)
    100,                      // output channel buffer size
    5 * time.Second,          // idle timeout before scale down
)
```

### Lifecycle

```
pool, err := parallel.NewPool(cfg)
if err != nil {
    log.Fatal(err)
}
pool.Start()
defer pool.Stop() // graceful shutdown
```

### Submit a task

```
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

### Read results

```
for result := range pool.GetOutput() {
    fmt.Println(result.GetResult())
    if err := result.GetError(); err != nil {
        // task failed
    }
}
```

### Suspend / Resume

```
pool.Suspend()   // reject new submissions, active tasks continue
// ... later
pool.Start()     // resume accepting tasks
```

### Update config at runtime

```
newCfg := parallel.NewPoolConfig(
    parallel.SendErrOnBusy, // change busy strategy
    parallel.SkipOutput,    // change output strategy
    2, 20, 200, 3*time.Second,
)
_ = pool.UpdateCfg(newCfg)
```

### Busy strategies (when all workers are busy)

| Strategy | Behaviour |
| --- | --- |
| `WaitOnBusy` | `Submit()` blocks until a worker becomes available or pool stops |
| `SendErrOnBusy` | `Submit()` returns `TaskBusyErr` immediately |
| `SilentSkipOnBusy` | `Submit()` returns `nil` (task silently dropped) |

### Output strategies (result delivery)

| Strategy | Behaviour |
| --- | --- |
| `WaitOutput` | Worker blocks until result is written to output channel |
| `SkipOutput` | Non‑blocking send; result is dropped if output channel is full |

### Errors returned by Pool

*   `TaskPanicErr` – task panicked (stack trace included)
*   `TaskContextErr` – task context already cancelled
*   `TaskBusyErr` – pool full and strategy `SendErrOnBusy`
*   `PoolCfgValidationErr` – invalid configuration
*   `PoolIsNotRunningErr` – pool not in `Running` state

## 🧩 Utilities – channels & slices

### `HandleParallelChan`

Process items from a channel with `threadCnt` workers, returns a buffered result channel.

```
in := make(chan int)
go func() {
    for i := 0; i < 20; i++ { in <- i }
    close(in)
}()

out := parallel.HandleParallelChan(4, in, func(n int) parallel.TaskResult[string] {
    return parallel.TaskResult[string]{Value: fmt.Sprintf("n=%d", n)}
})

for res := range out {
    fmt.Println(res.Value)
}
```

### `HandleParallelOut`

Process items and immediately handle each result with a callback. Returns a `WaitGroup` that waits for both processing and handling.

```
wg := parallel.HandleParallelOut(3, in,
    func(n int) parallel.TaskResult[int] { return parallel.TaskResult[int]{Value: n * 2} },
    func(res parallel.TaskResult[int]) { fmt.Println(res.Value) },
)
wg.Wait()
```

### `HandleParallelArr`

Process a slice concurrently with `threadCnt` workers, preserving original order. Supports cancellation via `ctx`.

```
ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
defer cancel()

input := []int{1,2,3,4,5}
results := parallel.HandleParallelArr(ctx, 4, input, func(n int) parallel.TaskResult[int] {
    time.Sleep(10 * time.Millisecond)
    return parallel.TaskResult[int]{Value: n * 10}
})

for i, r := range results {
    fmt.Printf("[%d] = %d\n", i, r.Value)
}
```

## 🔀 MergeChanWithBuffer

Merge multiple channels of the same type into one. Handles `nil` channels gracefully and returns a buffered channel that is closed when all input channels are closed and drained.

```
ch1 := make(chan int)
ch2 := make(chan int)
// fill and close both channels

merged := parallel.MergeChanWithBuffer(100, ch1, ch2, nil) // nil is skipped

for v := range merged {
    fmt.Println(v)
}
```

## 🧩 Generic types – `TaskResult` & `Task`

```
type TaskResult[T any] struct {
    Value T
    Err   error
}

type Task[T any] func(ctx context.Context) TaskResult[T]
```

These types are used by all the `HandleParallel*` functions, making them fully generic and type‑safe. You can also use them independently in your own goroutine management.

## 📐 Full examples

### Example 1: Using Pool

```
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

### Example 2: Parallel channel processing

```
func processNumbers() {
    in := make(chan int, 20)
    for i := 0; i < 20; i++ {
        in <- i
    }
    close(in)

    out := parallel.HandleParallelChan(5, in, func(n int) parallel.TaskResult[int] {
        return parallel.TaskResult[int]{Value: n * n}
    })

    for res := range out {
        fmt.Println("square:", res.Value)
    }
}
```

## 📌 Best practices

*   **Choose the right busy strategy** – `WaitOnBusy` provides backpressure, `SendErrOnBusy` gives immediate feedback, `SilentSkipOnBusy` is for non‑critical workloads.
*   **Match output strategy to consumer speed** – `WaitOutput` ensures no loss but may block workers; `SkipOutput` prevents worker blocking but may drop results.
*   **Always set idle timeout** – prevents worker bloat during low load.
*   **Use context cancellation** – tasks should check `ctx.Done()` to support graceful shutdown.
*   **Recover panics** – the pool already recovers and returns `TaskPanicErr` with a stack trace.
*   **Restart after Stop** – `Start()` recreates channels, so you can reuse the pool.
*   **For slice processing** – use `HandleParallelArr` when you need to preserve order; it's built for that.
*   **For streaming data** – use `HandleParallelChan` or `HandleParallelOut`.

## 🧪 Testing

The package has 100% test coverage (including race detection). Run:

```
go test -race -v ./...
```

GNU General Public License v3.0 · Built with ❤️ for Go 1.24+

[GPLv3 License](https://www.gnu.org/licenses/gpl-3.0.en.html)