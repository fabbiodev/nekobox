//go:build linux || freebsd

package main

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

func waitForParent(pid int, timeout time.Duration) error {
	if pid <= 0 {
		return nil
	}
	if pid == os.Getpid() {
		return fmt.Errorf("cannot wait for updater itself")
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("check old application: %w", err)
		}
		select {
		case <-deadline.C:
			return fmt.Errorf("old application did not exit; installation aborted")
		case <-tick.C:
		}
	}
}
