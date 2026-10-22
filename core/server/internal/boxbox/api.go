package boxbox

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func (s *Box) BeginClose(cancel context.CancelFunc, logFunc func(v ...any)) <-chan struct{} {
	return s.closeTask.Begin(func() error {
		start := time.Now()
		if cancel != nil {
			cancel()
		}
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
