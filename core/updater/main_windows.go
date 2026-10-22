package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func Launch(path string, args ...string) error {
	cmd := exec.Command(path, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

func run() error {
	version := flag.String("version", "", "version")
	chocolatey := flag.String("chocolatey_source", "", "Chocolatey source")
	winget := flag.Bool("winget_install", false, "install with winget")
	flag.Bool("verbose", false, "verbose mode")
	name := flag.String("name", "nekobox", "software name")
	digest := flag.String("sha256", "", "expected package SHA-256")
	flag.Parse()
	args := flag.Args()
	pkg, install, err := validateArguments(args, *winget)
	if err != nil {
		return err
	}
	if *winget {
		if *version == "" {
			return fmt.Errorf("winget requires a version")
		}
		err = Launch("winget", "install", "--exact", "--id", pkg, "--version", *version,
			"--override", "/S /WINGET=1 /UNPACK=1 /D="+install)
	} else {
		if !strings.EqualFold(filepath.Ext(pkg), ".exe") {
			return fmt.Errorf("expected a Windows installer executable")
		}
		if err = verifyPackage(pkg, *digest); err != nil {
			return err
		}
		chocoFlag := "/CHOCOLATEY=0"
		if *chocolatey != "" {
			if *version == "" || *name == "" {
				return fmt.Errorf("Chocolatey requires package name and version")
			}
			// Arguments are passed directly, never interpolated into PowerShell.
			if err = Launch("choco", "install", *name, "--version", *version, "--source", *chocolatey, "--skip-scripts", "-y"); err != nil {
				return err
			}
			chocoFlag = "/CHOCOLATEY=1"
		}
		err = Launch(pkg, "/S", "/UNPACK=1", chocoFlag, "/D="+install)
	}
	if err != nil {
		return fmt.Errorf("installer failed (application will not be relaunched): %w", err)
	}
	app := filepath.Join(install, "nekobox.exe")
	if info, err := os.Stat(app); err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("installed application is missing: %s", app)
	}
	cmd := exec.Command(app, args[2:]...)
	cmd.Dir = install
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start updated application: %w", err)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Print(err)
		MessageBoxPlain("NekoBox Updater", err.Error())
		os.Exit(1)
	}
}
