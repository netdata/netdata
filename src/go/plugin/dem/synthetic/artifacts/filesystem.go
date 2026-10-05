// SPDX-License-Identifier: GPL-3.0-or-later

package artifacts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func syncExternalDir(name string) error {
	file, err := os.Open(name)
	if err != nil {
		return err
	}
	return errors.Join(file.Sync(), file.Close())
}

func treeBytes(root *os.Root, dir string) (int64, error) {
	var bytes int64
	err := fs.WalkDir(root.FS(), dir, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		bytes += info.Size()
		return nil
	})
	return bytes, err
}

// Each newly created ancestor needs its parent entry synced on first use.
func makeStoreDir(name string) error {
	var dirs []string
	for current := name; ; current = filepath.Dir(current) {
		_, err := os.Stat(current)
		if err == nil {
			dirs = append(dirs, current)
			break
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		dirs = append(dirs, current)
		if filepath.Dir(current) == current {
			return err
		}
	}
	if err := os.MkdirAll(name, 0700); err != nil {
		return err
	}
	for _, dir := range dirs {
		f, err := os.Open(dir)
		if err != nil {
			return err
		}
		if err = errors.Join(f.Sync(), f.Close()); err != nil {
			return err
		}
	}
	return nil
}

func openRegular(root *os.Root, name string) (*os.File, error) {
	if !fs.ValidPath(name) {
		return nil, ErrInvalidCapture
	}
	parts := strings.Split(name, "/")
	for i := range parts {
		info, err := root.Lstat(strings.Join(parts[:i+1], "/"))
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("symbolic links are not artifacts")
		}
		if i < len(parts)-1 && !info.IsDir() {
			return nil, fmt.Errorf("artifact parent is not a directory")
		}
		if i == len(parts)-1 && !info.Mode().IsRegular() {
			return nil, fmt.Errorf("artifact is not a regular file")
		}
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("artifact is not a regular file: %v", err)
	}
	return file, nil
}

func writeSync(root *os.Root, name string, raw []byte) error {
	file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(raw)
	return errors.Join(writeErr, file.Sync(), file.Close())
}

func syncDir(root *os.Root, name string) error {
	file, err := root.Open(name)
	if err != nil {
		return err
	}
	return errors.Join(file.Sync(), file.Close())
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
