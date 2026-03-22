package tasks

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewTaskManager(t *testing.T) {
	tm := NewTaskManager()
	if tm == nil {
		t.Fatal("NewTaskManager returned nil")
	}
	if len(tm.tasks) != 0 {
		t.Fatal("New task manager should have empty tasks")
	}
}

func TestNewTask(t *testing.T) {
	tm := NewTaskManager()
	var executed int32

	task := tm.NewTask(func(ctx context.Context) {
		atomic.AddInt32(&executed, 1)
	})

	if task == nil {
		t.Fatal("NewTask returned nil")
	}

	// Wait for task to complete
	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt32(&executed) != 1 {
		t.Fatal("Task function was not executed")
	}
}

func TestNewTaskWithContext(t *testing.T) {
	tm := NewTaskManager()
	var cancelled int32

	task := tm.NewTask(func(ctx context.Context) {
		<-ctx.Done()
		atomic.AddInt32(&cancelled, 1)
	})

	// Cancel the task
	task.cancel()

	// Wait for cancellation
	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt32(&cancelled) != 1 {
		t.Fatal("Task was not cancelled")
	}
}

func TestStopAll(t *testing.T) {
	tm := NewTaskManager()
	var cancelled int32

	// Create multiple tasks
	for i := 0; i < 3; i++ {
		tm.NewTask(func(ctx context.Context) {
			<-ctx.Done()
			atomic.AddInt32(&cancelled, 1)
		})
	}

	time.Sleep(50 * time.Millisecond)

	// Stop all tasks
	tm.StopAll()

	// Wait for cancellations
	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt32(&cancelled) != 3 {
		t.Fatalf("Expected 3 cancelled tasks, got %d", cancelled)
	}

	if len(tm.tasks) != 0 {
		t.Fatal("Tasks slice should be empty after StopAll")
	}
}
