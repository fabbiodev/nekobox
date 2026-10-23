package boxbox

import (
	"context"
	"errors"
	"nekobox_core/internal/coreinclude"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/option"
)

type slowCloseService struct {
	release chan struct{}
	calls   atomic.Int32
}

func (*slowCloseService) Name() string                   { return "slow-close-test" }
func (*slowCloseService) Start(adapter.StartStage) error { return nil }
func (s *slowCloseService) Close() error                 { s.calls.Add(1); <-s.release; return nil }

func TestCloseTimeoutRemainsPendingAndIdempotent(t *testing.T) {
	instance, err := New(Options{
		Context: coreinclude.Context(context.Background()),
		Options: option.Options{Log: &option.LogOptions{Disabled: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = instance.Start(); err != nil {
		t.Fatal(err)
	}
	slow := &slowCloseService{release: make(chan struct{})}
	instance.internalService = append(instance.internalService, slow)
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(slow.release) }) }
	t.Cleanup(func() {
		release()
		if err := instance.CloseWithTimeout(nil, 5*time.Second, func(...any) {}); err != nil {
			t.Error(err)
		}
	})
	var cancelCalls atomic.Int32
	cancel := func() { cancelCalls.Add(1) }
	discard := func(...any) {}
	done := instance.BeginClose(cancel, discard)
	// No sleep or scheduler deadline: cancellation is a synchronous contract.
	if !errors.Is(instance.Context().Err(), context.Canceled) || cancelCalls.Load() != 1 {
		t.Fatal("BeginClose returned before cancelling background tasks")
	}
	if err = instance.CloseWithTimeout(cancel, time.Millisecond, discard); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("slow close reported success: %v", err)
	}
	if !errors.Is(instance.Context().Err(), context.Canceled) {
		t.Fatal("shutdown did not cancel background tasks")
	}
	select {
	case <-done:
		t.Fatal("close completed before resources were released")
	default:
	}
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e := instance.CloseWithTimeout(cancel, time.Second, discard); e != nil {
				t.Error(e)
			}
		}()
	}
	release()
	wg.Wait()
	if err = instance.Close(); err != nil {
		t.Fatal(err)
	}
	if slow.calls.Load() != 1 || cancelCalls.Load() != 1 {
		t.Fatal("duplicate close/cancel")
	}
}
