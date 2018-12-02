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
	"sync/atomic"
	"testing"
	"time"
)

func TestSubmit(t *testing.T) {
	p := NewPool(&Config{Size: 10, UnstoppableWorkers: 5})
	var count uint32
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		task := NewCustomTask(func(args ...interface{}) (interface{}, error) {
			atomic.AddUint32(&count, 1)
			wg.Done()
			return nil, nil
		})

		p.Submit(task)
	}

	activeCount := p.ActiveCount()
	if activeCount == 0 {
		t.Fatalf("invalid active workers value: %d; want > 0", activeCount)
	}

	wg.Wait()

	p.Close()

	if count != 100 {
		t.Fatalf("invalid count value: %d; want: 100", count)
	}
}

func TestSubmitClose1(t *testing.T) {
	p := NewPool(&Config{Size: 10, UnstoppableWorkers: 5})
	var count uint32
	completion := make(chan int, 10)

	for i := 0; i < 100; i++ {
		task := NewCustomTask(func(args ...interface{}) (interface{}, error) {
			i := args[0].(int)
			select {
			case completion <- i:
				break
			case <-p.close:
				return nil, nil
			}
			atomic.AddUint32(&count, 1)
			return nil, nil
		}, i)

		p.Submit(task)
	}

	resultCount := 0
LOOP:
	for {
		select {
		case <-completion:
			resultCount++
			if resultCount == 7 {
				break LOOP
			}
		}
	}

	p.Close()

	if count == 100 {
		t.Fatalf("invalid count value: 100; want: <100")
	}

	taskQueue := p.TaskQueue()

	size := taskQueue.Size()

	if size == 0 {
		t.Fatalf("invalid taskQueue size: 0; want >0")
	}

}

func TestSubmitClose2(t *testing.T) {
	p := NewPool(&Config{Size: 10, UnstoppableWorkers: 5})
	var count uint32
	completion := make(chan int, 10)

	for i := 0; i < 100; i++ {
		task := NewCustomTask(func(args ...interface{}) (interface{}, error) {
			i := args[0].(int)
			select {
			case completion <- i:
				break
			case <-p.close:
				return nil, nil
			}
			atomic.AddUint32(&count, 1)
			return nil, nil
		}, i)

		p.Submit(task)
	}

	resultCount := 0

LOOP:
	for {
		select {
		case <-completion:
			resultCount++
			if resultCount == 2 {
				break LOOP
			}
		}
	}

	p.Close()

	if count == 100 {
		t.Fatalf("invalid count value: 100; want: <100")
	}

}

func TestSubmitWithCompletionClose1(t *testing.T) {
	p := NewPool(&Config{Size: 10, UnstoppableWorkers: 5})
	completion1 := make(chan int, 10)
	completion2 := make(chan Task, 10)

	for i := 0; i < 100; i++ {
		task := NewCustomTask(func(args ...interface{}) (interface{}, error) {
			i := args[0].(int)
			select {
			case completion1 <- i:
				break
			case <-p.close:
				break
			}
			return 1, nil
		}, i)

		p.SubmitWithCompletion(completion2, task)
	}

	resultCount := 0

LOOP1:
	for {
		select {
		case <-completion1:
			resultCount++
			if resultCount == 7 {
				break LOOP1
			}
		}
	}

	p.Close()

}

func TestSubmitWithCompletionClose2(t *testing.T) {
	p := NewPool(&Config{Size: 10, UnstoppableWorkers: 5})
	completion1 := make(chan int, 10)
	completion2 := make(chan Task, 10)

	for i := 0; i < 100; i++ {
		task := NewCustomTask(func(args ...interface{}) (interface{}, error) {
			i := args[0].(int)
			select {
			case completion1 <- i:
				break
			case <-p.close:
				break
			}
			return 1, nil
		}, i)

		p.SubmitWithCompletion(completion2, task)
	}

	resultCount := 0

LOOP1:
	for {
		select {
		case <-completion1:
			resultCount++
			if resultCount == 2 {
				break LOOP1
			}
		}
	}

	p.Close()
}

func TestSubmitWithCompletion(t *testing.T) {
	p := NewPool(&Config{Size: 10, UnstoppableWorkers: 5})
	completion := make(chan Task, 5)

	for i := 0; i < 100; i++ {
		task := NewCustomTask(func(args ...interface{}) (interface{}, error) {
			return 1, nil
		})

		p.SubmitWithCompletion(completion, task)
	}

	count := 0

	timeout := time.After(1 * time.Second)

LOOP:
	for {
		select {
		case result := <-completion:
			if result.Error() != nil {
				t.Errorf("unxpexcted error: %v", result.Error())
			}

			count += result.Get().(int)
			if count == 100 {
				break LOOP
			}
		case <-timeout:
			break LOOP
		}
	}

	p.Close()

	if count != 100 {
		t.Fatalf("invalid count value %d; want: 100", count)
	}
}

func TestSubmitWithCompletionPanic(t *testing.T) {
	p := NewPool(&Config{Size: 10, UnstoppableWorkers: 5})
	completion := make(chan Task, 5)

	for i := 1; i <= 100; i++ {
		task := NewCustomTask(func(args ...interface{}) (interface{}, error) {
			i := args[0].(int)

			if i%2 == 0 {
				panic("error")
			}

			return 1, nil
		}, i)

		p.SubmitWithCompletion(completion, task)
	}

	countPanic := 0
	countNoErr := 0

	timeout := time.After(1 * time.Second)

	count := 0

LOOP:
	for {
		select {
		case result := <-completion:
			count += 1
			if result.Panic() != nil {
				countPanic++
			} else if result.Error() != nil {
				t.Errorf("unxpexcted error: %v", result.Error())
			} else {
				countNoErr += result.Get().(int)
			}

			if count == 100 {
				break LOOP
			}
		case <-timeout:
			break LOOP
		}
	}

	p.Close()

	if countPanic != 50 {
		t.Fatalf("invalid countPanic value: %d; want: 50", countPanic)
	}

	if countNoErr != 50 {
		t.Fatalf("invalid countPanic value: %d; want: 50", countNoErr)
	}
}

func TestSubmitWithCancel(t *testing.T) {
	p := NewPool(&Config{Size: 10, UnstoppableWorkers: 5})
	ctx, cancel := context.WithCancel(context.Background())
	var count uint32
	var wg1 sync.WaitGroup
	wg1.Add(7)
	var wg2 sync.WaitGroup
	wg2.Add(1)

	for i := 0; i < 20; i++ {
		task := NewCustomTask(func(args ...interface{}) (interface{}, error) {
			i := args[0].(int)
			if i < 7 {
				wg1.Done()
			} else {
				wg2.Wait()
			}
			atomic.AddUint32(&count, 1)
			return nil, nil
		}, i)

		p.SubmitWithContext(ctx, task)
	}

	wg1.Wait()
	cancel()
	wg2.Done()

	p.Wait()

	if count == 100 {
		t.Fatalf("invalid count value 100; want: <100")
	}

	p.Close()

	p = NewPool(&Config{Size: 10, UnstoppableWorkers: 5})
	ctx, cancel = context.WithCancel(context.Background())

	count = 0
	wg1.Add(2)
	wg2.Add(1)
	for i := 0; i < 100; i++ {
		task := NewCustomTask(func(args ...interface{}) (interface{}, error) {
			i := args[0].(int)
			if i < 2 {
				wg1.Done()
			} else {
				wg2.Wait()
			}
			atomic.AddUint32(&count, 1)
			return nil, nil
		}, i)

		p.SubmitWithContext(ctx, task)
	}

	wg1.Wait()
	cancel()
	wg2.Done()

	p.Wait()

	if count == 100 {
		t.Fatalf("invalid count value: 100; want: <100")
	}

	p.Close()
}

func TestSubmitCustom1(t *testing.T) {
	p := NewPool(&Config{Size: 10, UnstoppableWorkers: 5})
	completion := make(chan Task, 10)
	ctx, cancel := context.WithCancel(context.Background())
	var wg1 sync.WaitGroup
	wg1.Add(7)
	var wg2 sync.WaitGroup
	wg2.Add(1)

	for i := 0; i < 100; i++ {
		task := NewCustomTask(func(args ...interface{}) (interface{}, error) {
			i := args[0].(int)
			if i < 7 {
				wg1.Done()
			} else {
				wg2.Wait()
			}
			return 1, nil
		}, i)

		p.SubmitCustom(ctx, completion, task)
	}

	wg1.Wait()
	cancel()
	wg2.Done()

	p.Close()
}

func TestSubmitCustom2(t *testing.T) {
	p := NewPool(&Config{Size: 10, UnstoppableWorkers: 5})
	ctx, cancel := context.WithCancel(context.Background())
	completion := make(chan Task, 10)
	var wg1 sync.WaitGroup
	wg1.Add(2)
	var wg2 sync.WaitGroup
	wg2.Add(1)

	for i := 0; i < 100; i++ {
		task := NewCustomTask(func(args ...interface{}) (interface{}, error) {
			i := args[0].(int)
			if i < 2 {
				wg1.Done()
			} else {
				wg2.Wait()
			}
			return 1, nil
		}, i)

		p.SubmitCustom(ctx, completion, task)
	}

	wg1.Wait()
	cancel()
	wg2.Done()

	p.Close()
}

func TestSubmitImmediate(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	p := NewPool(&Config{Size: 2, UnstoppableWorkers: 2})
	task := NewCustomTask(func(args ...interface{}) (interface{}, error) {
		wg.Done()
		return 1, nil
	})

	err := p.Submit(task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wg.Wait()

	task = NewCustomTask(func(args ...interface{}) (interface{}, error) {
		return 1, nil
	})

	err = p.SubmitImmediate(task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p.Close()
}

func TestSubmitImmediateError(t *testing.T) {
	var wg1 sync.WaitGroup
	wg1.Add(1)
	var wg2 sync.WaitGroup
	wg2.Add(1)
	p := NewPool(&Config{Size: 1, UnstoppableWorkers: 1})
	task := NewCustomTask(func(args ...interface{}) (interface{}, error) {
		wg2.Done()
		wg1.Wait()
		return 1, nil
	})

	err := p.Submit(task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	task = NewCustomTask(func(args ...interface{}) (interface{}, error) {
		return 1, nil
	})

	wg2.Wait()

	err = p.SubmitImmediate(task)
	if err != ErrPoolUnavailable {
		t.Fatalf("unexpected error: %v, want: %v", err, ErrPoolUnavailable)
	}

	wg1.Done()

	p.Close()
}

func TestSubmitImmediateCloseError(t *testing.T) {
	p := NewPool(&Config{Size: 1, UnstoppableWorkers: 1})

	task := NewCustomTask(func(args ...interface{}) (interface{}, error) {
		return 1, nil
	})

	p.Close()

	err := p.SubmitImmediate(task)
	if err != ErrPoolClosed {
		t.Fatalf("unexpected error: %v, want: %v", err, ErrPoolUnavailable)
	}

}

func TestWait(t *testing.T) {
	p := NewPool(&Config{Size: 10, UnstoppableWorkers: 5})
	var count uint32
	var wg sync.WaitGroup
	wg.Add(7)
	for i := 0; i < 100; i++ {
		task := NewCustomTask(func(args ...interface{}) (interface{}, error) {
			i := args[0].(int)
			if i < 7 {
				wg.Done()
			}
			atomic.AddUint32(&count, 1)
			return nil, nil
		}, i)

		p.Submit(task)
	}

	p.Wait()

	if count != 100 {
		t.Fatalf("invalid count value: %d; want: 100", count)
	}

	p.Close()
}

func TestWaitClose(t *testing.T) {
	p := NewPool(&Config{Size: 10, UnstoppableWorkers: 5})
	completion := make(chan Task, 10)
	for i := 0; i < 100; i++ {
		task := NewCustomTask(func(args ...interface{}) (interface{}, error) {
			return nil, nil
		})

		p.SubmitWithCompletion(completion, task)
	}

	resultCount := 0
LOOP:
	for {
		select {
		case <-completion:
			resultCount++
			if resultCount == 7 {
				break LOOP
			}
			break
		default:
			break LOOP
		}
	}

	p.Close()

	p.Wait()
}

func TestResize1(t *testing.T) {
	p := NewPool(&Config{Size: 10, UnstoppableWorkers: 5})

	var wg1 sync.WaitGroup
	wg1.Add(7)
	var wg2 sync.WaitGroup
	wg2.Add(1)
	for i := 0; i < 100; i++ {
		task := NewCustomTask(func(args ...interface{}) (interface{}, error) {
			i := args[0].(int)
			if i < 7 {
				wg1.Done()
			} else {
				wg2.Wait()
			}
			return nil, nil
		}, i)

		p.Submit(task)
	}

	wg1.Wait()

	p.SetSize(7)
	if p.config.Size != 7 {
		t.Fatalf("invalid pool size %d; want 7", p.Size())
	}

	p.SetSize(12)
	if p.config.Size != 12 {
		t.Fatalf("invalid pool size %d; want 12", p.Size())
	}

	p.SetSize(3)
	if p.config.Size != 3 {
		t.Fatalf("invalid pool size %d; want 3", p.Size())
	}

	wg2.Done()

	p.Wait()

	unstoppableWorkers := p.UnstoppableWorkers()
	if unstoppableWorkers != 3 {
		t.Fatalf("invalid unstoppableWorkers value %d; want 3", unstoppableWorkers)
	}

	p.Close()
}

func TestResize2(t *testing.T) {
	p := NewPool(&Config{Size: 10, UnstoppableWorkers: 5})

	var wg1 sync.WaitGroup
	wg1.Add(2)
	var wg2 sync.WaitGroup
	wg2.Add(1)
	for i := 0; i < 100; i++ {
		task := NewCustomTask(func(args ...interface{}) (interface{}, error) {
			i := args[0].(int)
			if i < 2 {
				wg1.Done()
			} else {
				wg2.Wait()
			}
			return nil, nil
		}, i)

		p.Submit(task)
	}

	wg1.Wait()

	p.SetSize(7)
	if p.config.Size != 7 {
		t.Fatalf("invalid pool size %d; want 7", p.Size())
	}

	p.SetSize(12)
	if p.config.Size != 12 {
		t.Fatalf("invalid pool size %d; want 12", p.Size())
	}

	p.SetSize(3)
	if p.config.Size != 3 {
		t.Fatalf("invalid pool size %d; want 3", p.Size())
	}

	wg2.Done()

	p.Wait()

	unstoppableWorkers := p.UnstoppableWorkers()
	if unstoppableWorkers != 3 {
		t.Fatalf("invalid unstoppableWorkers value %d; want 3", unstoppableWorkers)
	}

	p.Close()

}
