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
	handle, err := syscall.OpenProcess(0x00100000, false, uint32(pid)) // SYNCHRONIZE
	if errors.Is(err, syscall.Errno(87)) {                             // ERROR_INVALID_PARAMETER
		return nil
	} // Already exited.
	if err != nil {
		return fmt.Errorf("open old application: %w", err)
	}
	defer syscall.CloseHandle(handle)
	result, err := syscall.WaitForSingleObject(handle, uint32(timeout.Milliseconds()))
	if err != nil {
		return err
	}
	if result != syscall.WAIT_OBJECT_0 {
		return fmt.Errorf("old application did not exit; installation aborted")
	}
	return nil
}
