//go:build linux || freebsd

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func run() error {
	flag.Bool("verbose", false, "verbose mode")
	digest := flag.String("sha256", "", "expected package SHA-256")
	waitPID := flag.Int("waitpid", 0, "wait for old application to exit")
	readyFile := flag.String("ready-file", "", "acknowledge readiness before GUI exits")
	flag.Parse()
	args := flag.Args()
	pkg, install, err := validateArguments(args, false)
	if err != nil {
		return err
	}
	if err := verifyPackage(pkg, *digest); err != nil {
		return err
	}
	if err = acknowledgeReady(*readyFile); err != nil {
		return err
	}
	if *readyFile != "" {
		defer os.Remove(*readyFile)
	}
	if err = waitForParent(*waitPID, parentExitTimeout); err != nil {
		return err
	}
	if err = checkHandoffCancelled(*readyFile); err != nil {
		return err
	}
	if err := Updater(pkg, install); err != nil {
		return err
	}
	app := install
	info, err := os.Stat(install)
	if err != nil {
		return err
	}
	if info.IsDir() {
		app = filepath.Join(install, "nekobox")
	}
	cmd := exec.Command(app, args[2:]...)
	cmd.Dir = filepath.Dir(app)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start updated application: %w", err)
	}
	return nil
}

func main() {
	updaterMain(run)
	os.Exit(updaterExitCode)
}
