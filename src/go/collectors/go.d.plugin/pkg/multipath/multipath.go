// SPDX-License-Identifier: GPL-3.0-or-later

package multipath

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mitchellh/go-homedir"
)

type ErrNotFound struct{ msg string }

func (e ErrNotFound) Error() string { return e.msg }

// IsNotFound returns a boolean indicating whether the error is ErrNotFound or not.
func IsNotFound(err error) bool {
	switch err.(type) {
	case ErrNotFound:
		return true
	}
	return false
}

// MultiPath multi-paths
type MultiPath []string

// New multi-paths
func New(paths ...string) MultiPath {
	set := map[string]bool{}
	mPath := make(MultiPath, 0)

	for _, dir := range paths {
		if dir == "" {
			continue
		}
		if d, err := homedir.Expand(dir); err != nil {
			dir = d
		}
		if !set[dir] {
			mPath = append(mPath, dir)
			set[dir] = true
		}
	}

	return mPath
}

// Find finds a file in given paths
func (p MultiPath) Find(filename string) (string, error) {
	for _, dir := range p {
		file := filepath.Join(dir, filename)
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			return file, nil
		}
	}
	return "", ErrNotFound{msg: fmt.Sprintf("can't find '%s' in %v", filename, p)}
}

func (p MultiPath) FindFiles(suffix string) ([]string, error) {
	set := make(map[string]bool)
	var files []string

	for _, dir := range p {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if !e.Type().IsRegular() || !strings.HasSuffix(e.Name(), suffix) || set[e.Name()] {
				continue
			}
			set[e.Name()] = true

			name := filepath.Join(dir, e.Name())
			files = append(files, name)
		}
	}

	return files, nil
}
