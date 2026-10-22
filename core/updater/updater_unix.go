//go:build linux || freebsd

package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func Updater(pkg, install string) error {
	info, err := os.Stat(install)
	if err != nil {
		return err
	}
	// AppImage updates replace one executable, not a directory tree.
	parent := install
	if !info.IsDir() {
		parent = filepath.Dir(install)
	}
	stage, err := os.MkdirTemp(parent, ".nekobox-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if !info.IsDir() {
		if !strings.HasSuffix(pkg, ".AppImage") {
			return fmt.Errorf("expected an AppImage update")
		}
		in, err := os.Open(pkg)
		if err != nil {
			return err
		}
		defer in.Close()
		if err := writeStaged(stage, filepath.Base(install), in, 0755, infoSize(pkg)); err != nil {
			return err
		}
		return installTree(stage, parent, os.Rename)
	}
	if strings.HasSuffix(pkg, ".zip") {
		reader, err := zip.OpenReader(pkg)
		if err != nil {
			return err
		}
		defer reader.Close()
		var total int64
		for i, entry := range reader.File {
			if i > 20000 || entry.UncompressedSize64 > 1<<30 {
				return fmt.Errorf("update archive too large")
			}
			if entry.FileInfo().IsDir() {
				continue
			}
			if !entry.Mode().IsRegular() {
				return fmt.Errorf("non-regular update entry")
			}
			total += int64(entry.UncompressedSize64)
			if total > 2<<30 {
				return fmt.Errorf("update archive too large")
			}
			file, err := entry.Open()
			if err != nil {
				return err
			}
			err = writeStaged(stage, entry.Name, file, entry.Mode().Perm(), int64(entry.UncompressedSize64))
			file.Close()
			if err != nil {
				return err
			}
		}
	} else if strings.HasSuffix(pkg, ".tar.gz") || strings.HasSuffix(pkg, ".tgz") {
		file, err := os.Open(pkg)
		if err != nil {
			return err
		}
		defer file.Close()
		gz, err := gzip.NewReader(file)
		if err != nil {
			return err
		}
		defer gz.Close()
		reader := tar.NewReader(gz)
		var total int64
		for count := 0; ; count++ {
			header, err := reader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			total += header.Size
			if count > 20000 || header.Size < 0 || header.Size > 1<<30 || total > 2<<30 {
				return fmt.Errorf("update archive too large")
			}
			if header.Typeflag == tar.TypeDir {
				continue
			}
			if header.Typeflag != tar.TypeReg {
				return fmt.Errorf("non-regular update entry")
			}
			if err := writeStaged(stage, header.Name, reader, os.FileMode(header.Mode).Perm(), header.Size); err != nil {
				return err
			}
		}
	} else {
		return fmt.Errorf("unsupported update archive")
	}
	content := filepath.Join(stage, "nekobox")
	for _, name := range []string{"nekobox", "nekobox_core"} {
		path := filepath.Join(content, name)
		if fi, err := os.Stat(path); err != nil || !fi.Mode().IsRegular() || fi.Size() == 0 {
			return fmt.Errorf("incomplete update: missing %s", name)
		}
		if err := os.Chmod(path, 0755); err != nil {
			return err
		}
	}
	return installTree(content, install, os.Rename)
}

func infoSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return -1
	}
	return info.Size()
}

func writeStaged(stage, name string, reader io.Reader, mode os.FileMode, size int64) error {
	clean := filepath.Clean(filepath.FromSlash(name))
	if size < 0 || size > 1<<30 || filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("unsafe update entry: %s", name)
	}
	target := filepath.Join(stage, clean)
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode&0755)
	if err != nil {
		return err
	}
	n, err := io.CopyN(file, reader, size)
	if err == nil && n != size {
		err = io.ErrUnexpectedEOF
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}
