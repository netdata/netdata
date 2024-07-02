// SPDX-License-Identifier: GPL-3.0-or-later

package multipath

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mitchellh/go-homedir"
)

type ErrNotFound struct{ msg string }

func (e ErrNotFound) Error() string { return e.msg }

// IsNotFound returns a boolean indicating whether the error is ErrNotFound or not.
func IsNotFound(err error) bool {
	var errNotFound ErrNotFound
	return errors.As(err, &errNotFound)
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

func (p MultiPath) FindFiles(suffixes ...string) ([]string, error) {
	set := make(map[string]bool)
	var files []string

	for _, dir := range p {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if !e.Type().IsRegular() {
				continue
			}

			ext := filepath.Ext(e.Name())
			name := strings.TrimSuffix(e.Name(), ext)

			if (len(suffixes) != 0 && !slices.Contains(suffixes, ext)) || set[name] {
				continue
			}

			set[name] = true
			file := filepath.Join(dir, e.Name())
			files = append(files, file)
		}
	}

	return files, nil
}
