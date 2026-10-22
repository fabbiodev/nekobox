package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateValidation(t *testing.T) {
	dir := t.TempDir()
	pkg := filepath.Join(dir, "update.exe")
	if err := os.WriteFile(pkg, []byte("complete installer"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{nil, {pkg}, {pkg, "relative"}, {"missing", dir}, {dir, dir}, {pkg, filepath.VolumeName(dir) + string(filepath.Separator)}} {
		if _, _, err := validateArguments(args, false); err == nil {
			t.Fatalf("accepted invalid arguments: %v", args)
		}
	}
	if _, _, err := validateArguments([]string{pkg, dir}, false); err != nil {
		t.Fatal(err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte("complete installer")))
	if err := verifyPackage(pkg, hash); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "sha256:" + hash, "bad", fmt.Sprintf("%064d", 0)} {
		if err := verifyPackage(pkg, bad); err == nil {
			t.Fatal("accepted invalid digest")
		}
	}
	if err := os.WriteFile(pkg, []byte("truncated"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyPackage(pkg, hash); err == nil {
		t.Fatal("accepted corrupted package")
	}
}
