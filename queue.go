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
	"errors"
	"sync"
)

// Errors that are used throughout the Eva API.
var (
	ErrTaskQueueClosed = errors.New("task queue closed")
	ErrTaskQueueEmpty  = errors.New("task queue is empty")
)

// TaskQueue is interface that additionally supports operations that wait for the queue to
// become non-empty when retrieving a task, and wait for space to become
// available in the queue when storing a task.
type TaskQueue interface {
	// Put inserts the task into this queue, waiting if necessary for
	// space to become available.
	Put(t Task) error

	// Size returns the current number of tasks in the queue.
	Size() int

	// Take retrieves and removes the head of this queue, waiting
	// if necessary until an task becomes available.
	Take() (Task, error)
}

type customTaskDeque struct {
	cond   *sync.Cond
	tasks  []*wrappedTask
	closed bool
}

func newCustomTaskDeque() *customTaskDeque {
	return &customTaskDeque{
		cond: sync.NewCond(new(sync.Mutex)),
	}
}

func (td *customTaskDeque) close() {
	td.cond.L.Lock()
	defer td.cond.L.Unlock()

	if td.closed {
		return
	}

	td.closed = true

	td.cond.Broadcast()
}

func (td *customTaskDeque) take() (*wrappedTask, error) {
	td.cond.L.Lock()
	defer td.cond.L.Unlock()

	for len(td.tasks) == 0 && !td.closed {
		td.cond.Wait()
	}

	if td.closed {
		return nil, ErrTaskQueueClosed
	}

	var t *wrappedTask
	for i, task := range td.tasks {
		skipped := td.skipTask(task)
		if skipped {
			continue
		}
		t = task
		if len(td.tasks) == 1 {
			td.tasks = td.tasks[:0:0]
		} else {
			td.tasks = td.tasks[i+1:]
		}
		break
	}

	if t == nil {
		td.tasks = td.tasks[:0:0]
		return nil, ErrTaskQueueEmpty
	}

	return t, nil
}

func (td *customTaskDeque) put(task *wrappedTask, prepend bool) error {
	td.cond.L.Lock()
	defer td.cond.L.Unlock()

	if prepend {
		td.tasks = append([]*wrappedTask{task}, td.tasks...)
		return nil
	}

	if td.closed {
		return ErrTaskQueueClosed
	}

	td.tasks = append(td.tasks, task)
	td.cond.Signal()

	return nil
}

func (td *customTaskDeque) signal() {
	td.cond.L.Lock()
	defer td.cond.L.Unlock()

	if td.closed {
		return
	}

	td.cond.Signal()
}

func (td *customTaskDeque) skipTask(task *wrappedTask) bool {
	if task.ctx != nil {
		select {
		case <-task.ctx.Done():
			task.Cancel()
			task.onComplete()
			return true
		default:
			return false
		}
	}

	return false
}

// Put inserts the task into this queue, waiting if necessary for
// space to become available.
// It returns ErrTaskQueueClosed only if queue become closed.
// It returns ErrTaskQueueEmpty only if queue is empty.
func (td *customTaskDeque) Put(t Task) error {
	task := &wrappedTask{Task: t}
	return td.put(task, false)
}

// Size returns the current number of tasks in the queue.
func (td *customTaskDeque) Size() int {
	td.cond.L.Lock()
	defer td.cond.L.Unlock()

	return len(td.tasks)
}

// Take retrieves and removes the head of this queue, waiting
// if necessary until an task becomes available.
// It returns ErrTaskQueueClosed only if queue become closed.
func (td *customTaskDeque) Take() (Task, error) {
	return td.take()
}
