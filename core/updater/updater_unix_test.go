//go:build linux || freebsd

package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestRejectUnsafeArchive(t *testing.T) {
	for _, name := range []string{"../escape", "/absolute", "nekobox/../../escape", "nekobox/only-one-file"} {
		dir := t.TempDir()
		pkg := filepath.Join(dir, "update.zip")
		f, err := os.Create(pkg)
		if err != nil {
			t.Fatal(err)
		}
		writer := zip.NewWriter(f)
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		entry.Write([]byte("invalid"))
		writer.Close()
		f.Close()
		install := filepath.Join(dir, "app")
		os.Mkdir(install, 0700)
		os.WriteFile(filepath.Join(install, "nekobox"), []byte("original"), 0700)
		if err := Updater(pkg, install); err == nil {
			t.Fatalf("accepted unsafe/incomplete archive: %s", name)
		}
		old, err := os.ReadFile(filepath.Join(install, "nekobox"))
		if err != nil || string(old) != "original" {
			t.Fatal("damaged old installation")
		}
	}
}

func TestInstallCompleteArchive(t *testing.T) {
	dir := t.TempDir()
	pkg := filepath.Join(dir, "update.zip")
	f, err := os.Create(pkg)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(f)
	for _, name := range []string{"nekobox/nekobox", "nekobox/nekobox_core", "nekobox/settings/user.db"} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte("new")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	install := filepath.Join(dir, "app")
	os.MkdirAll(filepath.Join(install, "settings"), 0700)
	os.WriteFile(filepath.Join(install, "settings", "user.db"), []byte("preserved"), 0600)
	if err := Updater(pkg, install); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(install, "nekobox"))
	if err != nil || string(data) != "new" {
		t.Fatalf("new application missing: %v", err)
	}
	data, err = os.ReadFile(filepath.Join(install, "settings", "user.db"))
	if err != nil || string(data) != "preserved" {
		t.Fatal("settings overwritten")
	}
}
