package lifecycle

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTimeoutDoesNotReleaseResources(t *testing.T) {
	var task CloseTask
	release := make(chan struct{})
	var calls atomic.Int32
	fn := func() error { calls.Add(1); <-release; return nil }
	done := task.Begin(fn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if err := task.Wait(ctx, fn); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline, got %v", err)
	}
	select {
	case <-done:
		t.Fatal("timeout released resources still in use")
	default:
	}
	close(release)
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if err := task.Wait(ctx2, fn); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatal("close was started more than once")
	}
}

func TestConcurrentWaitersShareCloseAndError(t *testing.T) {
	var task CloseTask
	var calls atomic.Int32
	want := errors.New("close failed")
	release := make(chan struct{})
	fn := func() error { calls.Add(1); <-release; return want }
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := task.Wait(context.Background(), fn); !errors.Is(err, want) {
				t.Errorf("wrong close result: %v", err)
			}
		}()
	}
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("close called %d times", calls.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := task.Wait(ctx, fn); !errors.Is(err, want) {
		t.Fatalf("completed result must win over expired context: %v", err)
	}
}
