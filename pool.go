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
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

// Constants that are used throughout the Eva API.
const (
	DefaultSize = 10
)

// Errors that are used throughout the Eva API.
var (
	ErrPoolSize           = errors.New("pool size less than or equal 0")
	ErrUnstoppableWorkers = errors.New("unstoppable workers count less than 0")
	ErrPoolClosed         = errors.New("pool closed")
	ErrPoolUnavailable    = errors.New("pool temporarily unavailable")
)

//Config is pool configuration
type Config struct {
	//Maximum number of workers that could be spawned.
	// It includes ActiveWorkers count.
	// If size == 0, then size poll change set to default (10)
	Size int
	// Number of workers that are always running.
	// If UnstoppableWorkers > Size, the Size will be overwritten
	UnstoppableWorkers int
}

// Pool is a struct that manages a collection of workers, each with their own
// goroutine. The Pool can initialize, expand, compress and close the workers,
// as well as processing task.
type Pool struct {
	config     Config
	close      chan struct{}
	syncClose  sync.Once
	taskQueue  *customTaskQueue
	arbiterWg  sync.WaitGroup
	workerWg   sync.WaitGroup
	waitWg     sync.WaitGroup
	workers    []*customWorker
	waitChan   chan struct{}
	spawnCount int64
	guard      sync.Mutex
}

func (c *Config) withDefaults() (config Config) {
	if c != nil {
		config = *c
	}

	if config.Size <= 0 {
		config.Size = DefaultSize
	}

	if config.UnstoppableWorkers < 0 {
		config.UnstoppableWorkers = 0
	}

	if config.UnstoppableWorkers > config.Size {
		config.Size = config.UnstoppableWorkers
	}

	return config
}

// NewPool is constructor for Pool
func NewPool(c *Config) *Pool {
	cp := new(Pool)

	cp.config = c.withDefaults()

	cp.close = make(chan struct{})
	cp.taskQueue = newTaskQueue()
	cp.workers = make([]*customWorker, 0, cp.config.UnstoppableWorkers)
	cp.waitChan = make(chan struct{}, 1)

	cp.workerWg.Add(cp.config.UnstoppableWorkers)
	for i := 0; i < cp.config.UnstoppableWorkers; i++ {
		worker := newCustomWorker(cp.close)
		cp.workers = append(cp.workers, worker)
		go worker.Run(func() {
			cp.waitTask()
			cp.taskQueue.signal()
		}, cp.workerWg.Done)
	}

	cp.arbiterWg.Add(1)

	go cp.arbiter()

	return cp
}

func (p *Pool) arbiter() {
	for {
		t, err := p.taskQueue.take()
		if err != nil {
			if err == ErrTaskQueueClosed {
				p.arbiterWg.Done()
				return
			} else if err == ErrTaskQueueEmpty {
				continue
			} else {
				panic(fmt.Errorf("unxpected task queue take error: %v", err))
			}
		}

		for {
			if p.submit(t) {
				break
			}

			select {
			case <-p.waitChan:
				break
			case <-p.close:
				p.taskQueue.put(t, true)
				p.arbiterWg.Done()
				return
			}
		}
	}
}

func (p *Pool) deleteWorker(w *customWorker) {
	for i, worker := range p.workers {
		if worker == w {
			copy(p.workers[i:], p.workers[i+1:])
			p.workers[len(p.workers)-1] = nil
			p.workers = p.workers[:len(p.workers)-1]
			return
		}
	}
}

func (p *Pool) setUnstoppableWorkers(count int) {
	if count < p.config.UnstoppableWorkers {
		workers := make([]*customWorker, len(p.workers))
		copy(workers, p.workers)
		for i := 0; i < count; i++ {
			workers[i].Release()
			for task := range workers[i].taskChan {
				p.taskQueue.put(task, true)
			}
			p.deleteWorker(workers[i])
		}
	} else {
		for i := 0; i < p.config.UnstoppableWorkers-count; i++ {
			p.workerWg.Add(p.config.UnstoppableWorkers - count)
			for i := 0; i < p.config.UnstoppableWorkers; i++ {
				worker := newCustomWorker(p.close)
				p.workers = append(p.workers, worker)
				go worker.Run(func() {
					p.waitTask()
					p.taskQueue.signal()
				}, p.workerWg.Done)
			}
		}
	}

	p.config.UnstoppableWorkers = count
}

func (p *Pool) spawn(t *wrappedTask) bool {
	if p.config.Size-p.config.UnstoppableWorkers == 0 {
		return false
	}

	availableSpawnCount := int64(p.config.Size - p.config.UnstoppableWorkers)

	if atomic.LoadInt64(&p.spawnCount) >= availableSpawnCount {
		return false
	}

	atomic.AddInt64(&p.spawnCount, 1)
	worker := newCustomWorker(p.close)
	go worker.Spawn(t, func() {
		atomic.AddInt64(&p.spawnCount, -1)
		p.waitTask()
		p.taskQueue.signal()
	})

	return true
}

func (p *Pool) submit(t *wrappedTask) bool {
	p.guard.Lock()
	defer p.guard.Unlock()
	if p.IsClosed() {
		return false
	}

	for _, worker := range p.workers {
		if worker.submit(t) {
			return true
		}
	}

	if p.spawn(t) {
		return true
	}

	return false
}

func (p *Pool) submitWrapped(t *wrappedTask) error {
	if t.ctx != nil {
		select {
		case <-t.ctx.Done():
			return t.ctx.Err()
		default:
			break
		}
	}
	p.guard.Lock()
	defer p.guard.Unlock()
	if p.IsClosed() {
		return ErrPoolClosed
	}

	t.onComplete = p.waitWg.Done

	p.waitWg.Add(1)

	err := p.taskQueue.put(t, false)
	if err != nil {
		p.waitWg.Done()
		return ErrPoolClosed
	}

	return nil
}

func (p *Pool) waitTask() {
	select {
	case p.waitChan <- struct{}{}:
		break
	default:
		break
	}
}

// ActiveCount returns the approximate number of workers that are actively executing tasks or idle.
func (p *Pool) ActiveCount() int {
	p.guard.Lock()
	defer p.guard.Unlock()
	return len(p.workers) + int(atomic.LoadInt64(&p.spawnCount))
}

// UnstoppableWorkers returns the current number of workers in the pool.
func (p *Pool) UnstoppableWorkers() int {
	p.guard.Lock()
	defer p.guard.Unlock()
	return p.config.UnstoppableWorkers
}

// Close terminates all spawned goroutines.
func (p *Pool) Close() {
	p.syncClose.Do(func() {
		p.guard.Lock()
		close(p.close)
		p.guard.Unlock()
		p.taskQueue.close()

		p.arbiterWg.Wait()

		workers := make([]*customWorker, len(p.workers))
		copy(workers, p.workers)

		for _, worker := range workers {
			worker.Release()
			for task := range worker.taskChan {
				p.taskQueue.put(task, true)
			}
		}

		for range p.taskQueue.tasks {
			p.waitWg.Done()
		}

		p.Wait()

		p.workerWg.Wait()
	})
}

// IsClosed returns true if this pool has been closed.
func (p *Pool) IsClosed() bool {
	select {
	case <-p.close:
		return true
	default:
		return false
	}
}

// SetUnstoppableWorkers changes unstoppable workers count.
// It returns ErrPoolClosed only if pool become closed.
// It returns ErrUnstoppableWorkers only if count < 0.
func (p *Pool) SetUnstoppableWorkers(count int) error {
	if count < 0 {
		return ErrUnstoppableWorkers
	}

	p.guard.Lock()
	defer p.guard.Unlock()

	if p.IsClosed() {
		return ErrPoolClosed
	}

	if p.config.UnstoppableWorkers == count {
		return nil
	}

	p.setUnstoppableWorkers(count)

	return nil
}

// SetSize changes pool size.
// It returns ErrPoolClosed only if pool become closed.
// It returns ErrPoolSize only if size < 0.
func (p *Pool) SetSize(size int) error {
	if size <= 0 {
		return ErrPoolSize
	}

	p.guard.Lock()
	defer p.guard.Unlock()

	if p.IsClosed() {
		return ErrPoolClosed
	}

	if size == p.config.Size {
		return nil
	} else if size < p.config.Size && size < p.config.UnstoppableWorkers {
		p.setUnstoppableWorkers(size)
	}

	p.config.Size = size

	return nil
}

// Size returns the current number of workers in the pool.
func (p *Pool) Size() int {
	p.guard.Lock()
	defer p.guard.Unlock()
	return p.config.Size
}

// Submit makes task to be scheduled over pool's workers.
// It returns ErrPoolClosed only if pool become closed and task
// can not be executed over its workers.
func (p *Pool) Submit(t Task) error {
	task := &wrappedTask{Task: t}
	return p.submitWrapped(task)
}

// SubmitCustom is a combination of SubmitWithCompletion and SubmitWithContext.
func (p *Pool) SubmitCustom(ctx context.Context, completion chan<- Task, t Task) error {
	task := &wrappedTask{Task: t, ctx: ctx, completion: completion}
	return p.submitWrapped(task)
}

// SubmitImmediate makes task to be scheduled without waiting for free
// workers. That is, if all workers are busy and pool is not rubber, then
// ErrUnavailable is returned immediately. It returns ErrPoolClosed only
// if pool become closed and task can not be executed over its workers.
func (p *Pool) SubmitImmediate(t Task) error {
	p.guard.Lock()
	defer p.guard.Unlock()
	if p.IsClosed() {
		return ErrPoolClosed
	}

	task := &wrappedTask{Task: t, onComplete: p.waitWg.Done}

	p.waitWg.Add(1)

	for _, worker := range p.workers {
		if worker.submit(task) {
			return nil
		}
	}

	if p.spawn(task) {
		return nil
	}

	p.waitWg.Done()
	return ErrPoolUnavailable
}

// SubmitWithCompletion makes task to be scheduled over pool's workers
// with returning completed task by completion channel. It returns ErrPoolClosed
// only if pool become closed and task can not be executed over its workers.
// Be careful, may deadlock! You need read-loop from completion channel.
func (p *Pool) SubmitWithCompletion(completion chan<- Task, t Task) error {
	task := &wrappedTask{Task: t, completion: completion}
	return p.submitWrapped(task)
}

// SubmitWithContext makes task to be scheduled over pool's workers
// with context support. It returns ErrPoolClosed only if pool
// become closed and task can not be executed over its workers.
// It returns ctx.Err() error only available when ctx is done.
func (p *Pool) SubmitWithContext(ctx context.Context, t Task) error {
	task := &wrappedTask{Task: t, ctx: ctx}
	return p.submitWrapped(task)
}

// TaskQueue returns the task queue used by this pool.
func (p *Pool) TaskQueue() TaskQueue {
	return p.taskQueue
}

// Wait pool is done with all its tasks.
func (p *Pool) Wait() {
	p.waitWg.Wait()
}
