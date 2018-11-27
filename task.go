// Copyright (c) 2018 Stepan Pesternikov
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package eva

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
)

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

type wrappedTask struct {
	Task
	ctx        context.Context
	completion chan<- Task
	onComplete func()
}

// CustomTask (implement Task interface) for goroutines pool.
type CustomTask struct {
	c          Callable
	cancelSync sync.Once
	cancelChan chan struct{}
	doneChan   chan struct{}
	resultChan chan interface{}
	errorChan  chan error
	panicChan  chan interface{}
	result     interface{}
	error      error
	panic      interface{}
}

// NewCustomTask is constructor for CustomTask
func NewCustomTask(f func(...interface{}) (interface{}, error), args ...interface{}) *CustomTask {
	return &CustomTask{
		c:          NewCustomCallable(f, args...),
		cancelChan: make(chan struct{}),
		resultChan: make(chan interface{}, 1),
		errorChan:  make(chan error, 1),
		panicChan:  make(chan interface{}, 1),
		doneChan:   make(chan struct{}),
	}
}

// Cancel perform cancel task.
func (ct *CustomTask) Cancel() {
	ct.cancelSync.Do(func() {
		close(ct.cancelChan)
	})
}

// Error returns task Run error.
// Blocked until the task is done.
func (ct *CustomTask) Error() error {
	select {
	case err, ok := <-ct.errorChan:
		if !ok {
			break
		}
		ct.error = err
	case <-ct.cancelChan:
		break
	}
	return ct.error
}

// Get returns task Run result.
// Blocked until the task is done.
func (ct *CustomTask) Get() interface{} {
	select {
	case result, ok := <-ct.resultChan:
		if !ok {
			break
		}
		ct.result = result
	case <-ct.cancelChan:
		break
	}

	return ct.result
}

// IsCancelled returns true if this task has been canceled.
func (ct *CustomTask) IsCancelled() bool {
	select {
	case <-ct.cancelChan:
		return true
	default:
		return false
	}
}

// IsDone returns true if this task has been done.
func (ct *CustomTask) IsDone() bool {
	select {
	case <-ct.doneChan:
		return true
	default:
		return false
	}
}

// Panic returns task Run panic.
// Blocked until the task is done.
func (ct *CustomTask) Panic() interface{} {
	select {
	case pan, ok := <-ct.panicChan:
		if !ok {
			break
		}
		ct.panic = pan
	case <-ct.cancelChan:
		break
	}
	return ct.panic
}

// Run perform task.
func (ct *CustomTask) Run() {
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("Recovered:\n%v\nStack:\n%s", r, string(debug.Stack()))

			ct.resultChan <- nil
			ct.errorChan <- nil
			ct.panicChan <- err
		}
		close(ct.doneChan)
		close(ct.resultChan)
		close(ct.panicChan)
	}()

	ct.c.Call()
	ct.resultChan <- ct.c.Result()
	ct.errorChan <- ct.c.Error()
}
