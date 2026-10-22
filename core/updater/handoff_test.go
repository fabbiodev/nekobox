package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestReadinessIsExclusiveAndConfined(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "updater.exe")
	path := executable + ".ready"
	if err := acknowledgeReadyForExecutable(path, executable); err != nil {
		t.Fatal(err)
	}
	if err := acknowledgeReadyForExecutable(path, executable); err == nil {
		t.Fatal("overwrote ready file")
	}
	if err := acknowledgeReadyForExecutable(filepath.Join(t.TempDir(), "outside"), executable); err == nil {
		t.Fatal("accepted unrelated directory")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "ready\n" {
		t.Fatal("readiness file was damaged")
	}
}

func TestUpdaterWaitChild(t *testing.T) {
	if os.Getenv("NEKOBOX_WAIT_TEST_CHILD") == "1" {
		time.Sleep(150 * time.Millisecond)
	}
}

func TestWaitForParentExitAndTimeout(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, timeout := range []time.Duration{time.Millisecond, time.Second * 5} {
		cmd := exec.Command(executable, "-test.run=^TestUpdaterWaitChild$")
		cmd.Env = append(os.Environ(), "NEKOBOX_WAIT_TEST_CHILD=1")
		if err = cmd.Start(); err != nil {
			t.Fatal(err)
		}
		waited := make(chan error, 1)
		go func() { waited <- cmd.Wait() }() // Reap Unix children while polling their PID.
		err = waitForParent(cmd.Process.Pid, timeout)
		if timeout == time.Millisecond && err == nil {
			t.Error("failed to time out for live parent")
		}
		if timeout > time.Second && err != nil {
			t.Error(err)
		}
		if err = <-waited; err != nil {
			t.Fatal(err)
		}
	}
	if err = waitForParent(os.Getpid(), time.Millisecond); err == nil {
		t.Fatal("accepted self wait")
	}
	if err = waitForParent(0, time.Millisecond); err != nil {
		t.Fatal(err)
	}
}
