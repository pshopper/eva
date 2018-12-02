package eva

import "testing"

func TestCustomTaskCancel(t *testing.T) {
	task := NewCustomTask(func(i ...interface{}) (interface{}, error) {
		return nil, nil
	})

	task.Cancel()

	if !task.IsCancelled() {
		t.Fatalf("invalid task status; expect: is canceled")
	}
}

func TestCustomTaskDone(t *testing.T) {
	task := NewCustomTask(func(i ...interface{}) (interface{}, error) {
		return nil, nil
	})

	task.Run()

	if !task.IsDone() {
		t.Fatalf("invalid task status; want: is done")
	}
}
