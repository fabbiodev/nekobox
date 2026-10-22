package lifecycle

import (
	"context"
	"fmt"
	"sync"
)

// CloseTask keeps ownership of resources until shutdown really completes.
// A caller timing out does not make those resources safe to reuse.
type CloseTask struct {
	once sync.Once
	done chan struct{}
	err  error
}

func (t *CloseTask) Begin(fn func() error) <-chan struct{} {
	t.once.Do(func() {
		t.done = make(chan struct{})
		go func() { t.err = fn(); close(t.done) }()
	})
	return t.done
}

func (t *CloseTask) Wait(ctx context.Context, fn func() error) error {
	done := t.Begin(fn)
	select {
	case <-done:
		return t.err
	case <-ctx.Done():
		// Prefer a completed close when both cases became ready together.
		select {
		case <-done:
			return t.err
		default:
		}
		return fmt.Errorf("previous profile is still closing; retry after it finishes or restart the core: %w", ctx.Err())
	}
}
