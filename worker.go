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
	"sync"
)

type customWorker struct {
	taskChan    chan *wrappedTask
	freeChan    chan struct{}
	syncRelease sync.Once
	release     chan struct{}
	kill        <-chan struct{}
}

func newCustomWorker(kill <-chan struct{}) *customWorker {
	return &customWorker{
		taskChan: make(chan *wrappedTask, 1),
		release:  make(chan struct{}),
		freeChan: make(chan struct{}, 1),
		kill:     kill,
	}
}

func (cw *customWorker) completeTask(t *wrappedTask) {
	if t.completion == nil {
		return
	}
	ctx := context.Background()
	if t.ctx != nil {
		ctx = t.ctx
	}
	select {
	case t.completion <- t:
		break
	case <-cw.release:
		break
	case <-cw.kill:
		break
	case <-ctx.Done():
		break
	}
}

func (cw *customWorker) runTask(t *wrappedTask, onComplete func()) {
	t.Run()
	cw.completeTask(t)
	select {
	case <-cw.freeChan:
		break
	default:
	}
	t.onComplete()
	onComplete()
}

func (cw *customWorker) submit(t *wrappedTask) bool {
	select {
	case cw.freeChan <- struct{}{}:
		cw.taskChan <- t
		return true
	default:
		break
	}

	return false
}

// Release
func (cw *customWorker) Release() {
	cw.syncRelease.Do(func() {
		close(cw.release)
		close(cw.taskChan)
	})
}

func (cw *customWorker) Run(onComplete func(), onRelease func()) {
LOOP:
	for {
		select {
		case t, ok := <-cw.taskChan:
			if !ok {
				break LOOP
			}
			cw.runTask(t, onComplete)
		case <-cw.release:
			break LOOP
		case <-cw.kill:
			break LOOP
		}
	}

	onRelease()
}

func (cw *customWorker) Spawn(t *wrappedTask, onComplete func()) {
	cw.runTask(t, onComplete)
}
