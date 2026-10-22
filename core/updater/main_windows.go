package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func Launch(path string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Stdout, cmd.Stderr = log.Writer(), log.Writer()
	return cmd.Run()
}

func nsisCommandLine(path, install, chocoFlag string) string {
	// NSIS /D consumes the rest of the command line and MUST NOT be quoted,
	// even for Program Files. exec.Command's default argv quoting breaks it.
	return syscall.EscapeArg(path) + " /S /UNPACK=1 " + chocoFlag + " /D=" + install
}

func launchInstaller(path, install, chocoFlag string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, path)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CmdLine: nsisCommandLine(path, install, chocoFlag)}
	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		log.Printf("Installer: %s", output)
	}
	return err
}

func run() error {
	version := flag.String("version", "", "version")
	chocolatey := flag.String("chocolatey_source", "", "Chocolatey source")
	winget := flag.Bool("winget_install", false, "install with winget")
	flag.Bool("verbose", false, "verbose mode")
	name := flag.String("name", "nekobox", "software name")
	digest := flag.String("sha256", "", "expected package SHA-256")
	waitPID := flag.Int("waitpid", 0, "wait for old application to exit")
	readyFile := flag.String("ready-file", "", "acknowledge readiness before GUI exits")
	flag.Parse()
	args := flag.Args()
	pkg, install, err := validateArguments(args, *winget)
	if err != nil {
		return err
	}
	if !*winget {
		if !strings.EqualFold(filepath.Ext(pkg), ".exe") {
			return fmt.Errorf("expected a Windows installer executable")
		}
		if err = verifyPackage(pkg, *digest); err != nil {
			return err
		}
	}
	if strings.ContainsAny(install, "\"\r\n") {
		return fmt.Errorf("invalid Windows installation path")
	}
	if *winget && *version == "" {
		return fmt.Errorf("winget requires a version")
	}
	if *chocolatey != "" && (*version == "" || *name == "") {
		return fmt.Errorf("Chocolatey requires package name and version")
	}
	if err = acknowledgeReady(*readyFile); err != nil {
		return err
	}
	if *readyFile != "" {
		defer os.Remove(*readyFile)
	}
	log.Printf("Package verified; waiting for application pid=%d", *waitPID)
	if err = waitForParent(*waitPID, parentExitTimeout); err != nil {
		return err
	}
	if err = checkHandoffCancelled(*readyFile); err != nil {
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
		log.Printf("Installing into %s", install)
		err = launchInstaller(pkg, install, chocoFlag)
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
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start updated application: %w", err)
	}
	return nil
}

func main() {
	updaterMain(run)
	os.Exit(updaterExitCode)
}
