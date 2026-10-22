package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Keep the old files on the same filesystem. On a normal installation error
// restore every replaced file; preserve the backup if rollback itself fails.
func installTree(stage, root string, rename func(string, string) error) error {
	var err error
	root, err = filepath.Abs(root)
	if err != nil {
		return err
	}
	stage, err = filepath.Abs(stage)
	if err != nil {
		return err
	}
	var files []string
	err = filepath.WalkDir(stage, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(stage, path)
		if err != nil {
			return err
		}
		first := strings.Split(filepath.ToSlash(rel), "/")[0]
		if first == "settings" || first == "global.ini" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in update: %s", rel)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("non-regular update entry: %s", rel)
		}
		// Do not follow existing symlinks in any destination component.
		for part := filepath.Join(root, rel); ; part = filepath.Dir(part) {
			info, err := os.Lstat(part)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if err == nil && info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("symlink in installation: %s", part)
			}
			if part == root {
				break
			}
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return err
	}
	backup, err := os.MkdirTemp(root, ".nekobox-backup-")
	if err != nil {
		return err
	}
	type change struct {
		rel            string
		old, installed bool
	}
	var changes []change
	rollback := func(cause error) error {
		var failures []error
		for i := len(changes) - 1; i >= 0; i-- {
			c := changes[i]
			target := filepath.Join(root, c.rel)
			if c.installed {
				if err := os.Remove(target); err != nil {
					failures = append(failures, err)
					continue
				}
			}
			if c.old {
				if err := os.Rename(filepath.Join(backup, c.rel), target); err != nil {
					failures = append(failures, err)
				}
			}
		}
		if len(failures) > 0 {
			return fmt.Errorf("%w; rollback incomplete, backup retained at %s: %v", cause, backup, failures)
		}
		os.RemoveAll(backup) // only our own freshly-created temporary directory
		return fmt.Errorf("update rolled back: %w", cause)
	}
	for _, rel := range files {
		target := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return rollback(err)
		}
		c := change{rel: rel}
		if info, err := os.Lstat(target); err == nil {
			if !info.Mode().IsRegular() {
				return rollback(fmt.Errorf("destination is not a regular file: %s", target))
			}
			old := filepath.Join(backup, rel)
			if err := os.MkdirAll(filepath.Dir(old), 0755); err != nil {
				return rollback(err)
			}
			if err := rename(target, old); err != nil {
				return rollback(err)
			}
			c.old = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return rollback(err)
		}
		changes = append(changes, c)
		if err := rename(filepath.Join(stage, rel), target); err != nil {
			return rollback(err)
		}
		changes[len(changes)-1].installed = true
	}
	return os.RemoveAll(backup)
}
