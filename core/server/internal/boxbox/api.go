package boxbox

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func (s *Box) BeginClose(cancel context.CancelFunc, logFunc func(v ...any)) <-chan struct{} {
	// Signal shutdown before returning, even if the close worker has not been
	// scheduled yet. Only resource teardown belongs in the background worker.
	s.closeCancelOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		if cancel != nil {
			cancel()
		}
	})
	return s.closeTask.Begin(func() error {
		start := time.Now()
		err := s.Close()
		logFunc("[Info] sing-box closed in", fmt.Sprintf("%d ms", time.Since(start).Milliseconds()))
		return err
	})
}

func (s *Box) CloseWithTimeout(cancel context.CancelFunc, d time.Duration, logFunc func(v ...any)) error {
	s.BeginClose(cancel, logFunc)
	ctx, stop := context.WithTimeout(context.Background(), d)
	defer stop()
	err := s.closeTask.Wait(ctx, s.Close)
	if errors.Is(err, context.DeadlineExceeded) {
		logFunc("[Warning] sing-box is still closing; its TUN resources are not released yet.")
	}
	return err
}
