package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCancelledHandoffDoesNotAcknowledge(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "updater.exe")
	ready := executable + ".ready"
	if err := os.WriteFile(ready+".cancel", nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := acknowledgeReadyForExecutable(ready, executable); err == nil {
		t.Fatal("acknowledged cancelled update")
	}
	if _, err := os.Stat(ready); !os.IsNotExist(err) {
		t.Fatal("created ready file")
	}
}
