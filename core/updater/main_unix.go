//go:build linux || freebsd

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func run() error {
	flag.Bool("verbose", false, "verbose mode")
	digest := flag.String("sha256", "", "expected package SHA-256")
	flag.Parse()
	args := flag.Args()
	pkg, install, err := validateArguments(args, false)
	if err != nil {
		return err
	}
	if err := verifyPackage(pkg, *digest); err != nil {
		return err
	}
	time.Sleep(2 * time.Second)
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
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
