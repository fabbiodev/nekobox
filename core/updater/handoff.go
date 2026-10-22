package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func acknowledgeReady(path string) error {
	if path == "" {
		return nil
	} // Older GUI builds do not use a handshake.
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return acknowledgeReadyForExecutable(path, executable)
}

func acknowledgeReadyForExecutable(path, executable string) error {
	if !filepath.IsAbs(path) || !strings.EqualFold(filepath.Dir(filepath.Clean(path)), filepath.Dir(executable)) {
		return fmt.Errorf("ready file must be beside the staged updater")
	}
	if err := checkHandoffCancelled(path); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("acknowledge updater readiness: %w", err)
	}
	_, writeErr := file.WriteString("ready\n")
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func checkHandoffCancelled(readyFile string) error {
	if readyFile == "" {
		return nil
	}
	if _, err := os.Stat(readyFile + ".cancel"); err == nil {
		return fmt.Errorf("update handoff was cancelled; installation aborted")
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

func updaterMain(run func() error) {
	logPath := ""
	if cache, err := os.UserCacheDir(); err == nil {
		dir := filepath.Join(cache, "NekoBox", "logs")
		if os.MkdirAll(dir, 0700) == nil {
			path := filepath.Join(dir, "updater.log")
			if file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600); err == nil {
				defer file.Close()
				logPath = path
				// A Windows GUI-subsystem process may have an invalid stderr.
				// MultiWriter(stderr,file) would stop before writing the file.
				log.SetOutput(file)
			}
		}
	}
	log.Printf("Updater started, pid=%d", os.Getpid())
	if err := run(); err != nil {
		log.Print(err)
		message := err.Error()
		if logPath != "" {
			message += "\n\nLog: " + logPath
		}
		MessageBoxPlain("NekoBox Updater", message)
		// Let the deferred log close run before returning a failure status.
		updaterExitCode = 1
	} else {
		log.Print("Update completed")
	}
}

var updaterExitCode int

const parentExitTimeout = 2 * time.Minute
