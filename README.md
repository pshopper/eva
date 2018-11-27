# EVA

[![goreportcard for pshopper/eva][1]][2]
[![godoc for pshopper/eva][3]][4]
[![MIT Licence][5]][6]


Package EVA implements a fixed goroutine pool for managing and recycling
a massive number of goroutines with unlimited task queue, allowing developers
to limit the number of goroutines that created by your concurrent programs.
For example, when processing jobs from HTTP requests that are CPU heavy you
can create a pool with a size that matches your CPU count.

## Features:

- Friendly interfaces: submitting tasks, getting the number.
of running goroutines, readjusting capacity of pool dynamically, closing pool.
- Automatically managing and recycling a massive number of goroutines.
- Availability of overriding the Task or Callable interface.
- Unlimited task queue.
- Recover panic.


## Installation

```sh
    go get github.com/pshopper/eva
```

Or, using dep:

```sh
    dep ensure -add github.com/pshopper/eva
```

## Usage

### Async

#### Simple

```go

    config := &eva.Config{Size: 10, UnstoppableWorkers: 5}
    p := eva.NewPool(&config)

    t := eva.NewCustomTask(func(args ...interface{}) (interface{}, error) {
        // some work here...


        return result, err
    }, args)

    p.Submit(t)

    p.Wait()

    fmt.Printf("Task result=%v; error=%v; panic=%v, t.Get(), t.Error(), t.Panic())

    p.Close()

```

#### With completion

```go

    config := &eva.Config{Size: 10, UnstoppableWorkers: 5}
    p := eva.NewPool(&config)

    completion := make(chan eva.Task, 3)

    for i := 0; i < 100; i++ {
        t := eva.NewCustomTask(func(args ...interface{}) (interface{}, error) {
            // some work here...


            return result, err
        }, args)

        p.SubmitWithCompletion(t)
    }


    for i := 0; i < 100; i++ {
        select {
        case t := <-completion:
            fmt.Printf("Task result=%v; error=%v; panic=%v, t.Get(), t.Error(), t.Panic())
        }
    }


    p.Close()

```

#### With context

```go

    config := &eva.Config{Size: 10, UnstoppableWorkers: 5}
    p := eva.NewPool(&config)

    context, cancel := context.WithCancel(context.Background()) // WithDeadline or WithTimeout

    for i := 0; i < 100; i++ {
        t := eva.NewCustomTask(func(args ...interface{}) (interface{}, error) {
            // some work here...


                return result, err
        }, args)

        p.SubmitWithContext(context, t)
    }

    // some work here...

    cancel()

    p.Close()

```

#### With custom

```go

    config := &eva.Config{Size: 10, UnstoppableWorkers: 5}
    p := eva.NewPool(&config)

    context, cancel := context.WithCancel(context.Background()) // WithDeadline or WithTimeout
    completion := make(chan eva.Task, 3)

    for i := 0; i < 100; i++ {
        t := eva.NewCustomTask(func(args ...interface{}) (interface{}, error) {
            // some work here...


                return result, err
        }, args)

        p.SubmitCustom(context, completion, t)
    }

    for i := 0; i < 3; i++ {
        select {
        case t := <-completion:
            fmt.Printf("Task result=%v; error=%v; panic=%v, t.Get(), t.Error(), t.Panic())
        }
    }

    cancel()

    p.Close()

```

#### Immediate

```go

    config := &eva.Config{Size: 10, UnstoppableWorkers: 5}
    p := eva.NewPool(&config)

    t := eva.NewCustomTask(func(args ...interface{}) (interface{}, error) {
        // some work here...


        return result, err
    }, args)

    err := p.SubmitImmediate(t)

    if err == nil {
        fmt.Printf("Task result=%v; error=%v; panic=%v, t.Get(), t.Error(), t.Panic())
    }

    p.Close()

```

### Sync

```go

    config := &eva.Config{Size: 10, UnstoppableWorkers: 5}
    p := eva.NewPool(&config)

    t := eva.NewCustomTask(func(args ...interface{}) (interface{}, error) {
        // some work here...


        return result, err
    }, args)

    p.Submit(t)

    fmt.Printf("Task result=%v; error=%v; panic=%v, t.Get(), t.Error(), t.Panic()) // like CompletableFuture

    p.Close()

```

### Resize

You can change pool size:

```go
    pool.SetSize(1000)
    pool.SetSize(1000000)
```

Or you can change changes unstoppable workers count

```go
    pool.SetUnstoppableWorkers(1000)
    pool.SetUnstoppableWorkers(1000000)
```

## Interfaces

You can implement Task interface and submit it in pool:

```go

// Task interface for goroutines pool.
type Task interface {
	// Cancel perform cancel task.
	Cancel()

	// Error returns task Run error.
	// Blocked until the task is done.
	Error() error

	// Get returns task Run result.
	// Blocked until the task is done.
	Get() interface{}

	// IsCancelled returns true if this task has been canceled.
	IsCancelled() bool

	// IsDone returns true if this task has been done.
	IsDone() bool

	// Panic returns task Run panic.
	// Blocked until the task is done.
	Panic() interface{}

	// Run perform task.
	Run()
}

```

This is safe to perform from any goroutine even if others are still processing.

## Ordering

All the tasks submitted to EVA pool will not be guaranteed to be processed in order,
because those tasks distribute among a series of concurrent workers, thus those tasks are processed concurrently.

[1]: https://goreportcard.com/badge/github.com/pshopper/eva
[2]: https://goreportcard.com/report/github.com/pshopper/eva
[3]: https://godoc.org/github.com/pshopper/eva?status.svg
[4]: https://godoc.org/github.com/pshopper/eva
[5]: https://badges.frapsoft.com/os/mit/mit.svg?v=103
[6]: https://opensource.org/licenses/mit-license.php