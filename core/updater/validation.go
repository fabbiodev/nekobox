package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func validateArguments(args []string, packageManager bool) (string, string, error) {
	if len(args) < 2 {
		return "", "", errors.New("usage: updater [options] -- package absolute-install-path [application arguments]")
	}
	if !filepath.IsAbs(args[1]) {
		return "", "", errors.New("installation path must be absolute")
	}
	install := filepath.Clean(args[1])
	if install == filepath.VolumeName(install)+string(filepath.Separator) {
		return "", "", errors.New("refusing to install into a filesystem root")
	}
	if _, err := os.Stat(install); err != nil {
		return "", "", fmt.Errorf("installation path: %w", err)
	}
	pkg := args[0]
	if packageManager {
		if pkg == "" || strings.HasPrefix(pkg, "-") {
			return "", "", errors.New("invalid package identifier")
		}
	} else {
		var err error
		pkg, err = filepath.Abs(pkg)
		if err != nil {
			return "", "", err
		}
		info, err := os.Stat(pkg)
		if err != nil {
			return "", "", err
		}
		if !info.Mode().IsRegular() || info.Size() == 0 {
			return "", "", errors.New("update package must be a nonempty regular file")
		}
	}
	return pkg, install, nil
}

func verifyPackage(path, digest string) error {
	expected, err := hex.DecodeString(digest)
	if err != nil || len(expected) != sha256.Size {
		return errors.New("a valid SHA-256 digest is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != strings.ToLower(digest) {
		return errors.New("update SHA-256 mismatch; installation aborted")
	}
	return nil
}
