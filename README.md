# Parallel library

Contains classes and methods for async and parallel works with goroutines

```shell
go get -u github.com/devian2011/parallel 
```

## Methods

### Await

Make async action

```go
package main

import (
	"context"
	"fmt"

	"github.com/devian2011/parallel"
)

func main() {
	promise := parallel.Await(context.Background(), func() parallel.TaskResult {
		return &parallel.TaskResultImpl{
			value: "task result", // Execution result
			err:   nil, // Execution error
		}
	})

	fmt.Println("Some action")
	fmt.Println("Some action to")

	result := promise.Get()
	
	fmt.Println(result.GetValue().(string)) // Will print "task result"
	fmt.Println(result.HasError()) // Print false
}
```

### Parallel

Execute function in pool of goroutines

#### HandleParallelChan
```go
package example

func process() {
	
}

```

#### HandleParallelOut

```go

```

#### HandleParallelArr

```go

```

