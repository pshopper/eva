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

type customTaskQueue struct {
	cond   *sync.Cond
	tasks  []*wrappedTask
	closed bool
}

func newTaskQueue() *customTaskQueue {
	return &customTaskQueue{
		cond: sync.NewCond(new(sync.Mutex)),
	}
}

func (tq *customTaskQueue) close() {
	tq.cond.L.Lock()
	defer tq.cond.L.Unlock()

	if tq.closed {
		return
	}

	tq.closed = true

	tq.cond.Broadcast()
}

func (tq *customTaskQueue) take() (*wrappedTask, error) {
	tq.cond.L.Lock()
	defer tq.cond.L.Unlock()

	for len(tq.tasks) == 0 && !tq.closed {
		tq.cond.Wait()
	}

	if tq.closed {
		return nil, ErrTaskQueueClosed
	}

	var t *wrappedTask
	for i, task := range tq.tasks {
		skipped := tq.skipTask(task)
		if skipped {
			continue
		}
		t = task
		if len(tq.tasks) == 1 {
			tq.tasks = tq.tasks[:0:0]
		} else {
			tq.tasks = tq.tasks[i+1:]
		}
		break
	}

	if t == nil {
		tq.tasks = tq.tasks[:0:0]
		return nil, ErrTaskQueueEmpty
	}

	return t, nil
}

func (tq *customTaskQueue) put(task *wrappedTask, prepend bool) error {
	tq.cond.L.Lock()
	defer tq.cond.L.Unlock()

	if prepend {
		tq.tasks = append([]*wrappedTask{task}, tq.tasks...)
		return nil
	}

	if tq.closed {
		return ErrTaskQueueClosed
	}

	tq.tasks = append(tq.tasks, task)
	tq.cond.Signal()

	return nil
}

func (tq *customTaskQueue) signal() {
	tq.cond.L.Lock()
	defer tq.cond.L.Unlock()

	if tq.closed {
		return
	}

	tq.cond.Signal()
}

func (tq *customTaskQueue) skipTask(task *wrappedTask) bool {
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
func (tq *customTaskQueue) Put(t Task) error {
	task := &wrappedTask{Task: t}
	return tq.put(task, false)
}

// Size returns the current number of tasks in the queue.
func (tq *customTaskQueue) Size() int {
	tq.cond.L.Lock()
	defer tq.cond.L.Unlock()

	return len(tq.tasks)
}

// Take retrieves and removes the head of this queue, waiting
// if necessary until an task becomes available.
// It returns ErrTaskQueueClosed only if queue become closed.
func (tq *customTaskQueue) Take() (Task, error) {
	return tq.take()
}
