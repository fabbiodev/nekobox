package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallAndRollback(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "rollback"}[fail], func(t *testing.T) {
			root, stage := t.TempDir(), t.TempDir()
			for _, dir := range []string{root, stage} {
				os.Mkdir(filepath.Join(dir, "settings"), 0700)
				os.WriteFile(filepath.Join(dir, "settings", "user.db"), []byte("user data"), 0600)
			}
			for _, name := range []string{"nekobox", "nekobox_core"} {
				os.WriteFile(filepath.Join(root, name), []byte("old"), 0700)
				os.WriteFile(filepath.Join(stage, name), []byte("new"), 0700)
			}
			os.WriteFile(filepath.Join(stage, "settings", "user.db"), []byte("must not overwrite"), 0600)
			calls := 0
			err := installTree(stage, root, func(a, b string) error {
				calls++
				if fail && calls == 4 {
					return errors.New("injected disk/rename failure")
				}
				return os.Rename(a, b)
			})
			if (err != nil) != fail {
				t.Fatalf("unexpected result: %v", err)
			}
			expected := "new"
			if fail {
				expected = "old"
			}
			for _, name := range []string{"nekobox", "nekobox_core"} {
				data, err := os.ReadFile(filepath.Join(root, name))
				if err != nil || string(data) != expected {
					t.Fatalf("%s: %q %v", name, data, err)
				}
			}
			data, err := os.ReadFile(filepath.Join(root, "settings", "user.db"))
			if err != nil || string(data) != "user data" {
				t.Fatal("settings were modified")
			}
		})
	}
}
