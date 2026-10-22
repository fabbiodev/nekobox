package internal

import (
	"context"
	"nekobox_core/internal/boxbox"
	"sync"
	"testing"
)

func TestOldShutdownCannotClearNewGeneration(t *testing.T) {
	old, current := &boxbox.Box{}, &boxbox.Box{}
	SetInstance(old, func() {})
	SetInstance(current, func() {})
	ClearInstance(old)
	if CurrentBox() != current {
		t.Fatal("old shutdown removed the new profile")
	}
	ClearInstance(current)
	if CurrentBox() != nil {
		t.Fatal("instance not cleared")
	}
}

func TestConcurrentInstanceSnapshots(t *testing.T) {
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				instance := &boxbox.Box{}
				SetInstance(instance, context.CancelFunc(func() {}))
				_, _ = CurrentInstance()
				ClearInstance(instance)
			}
		}()
	}
	wg.Wait()
	ClearInstance(CurrentBox())
}
