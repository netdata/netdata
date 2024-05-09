// SPDX-License-Identifier: GPL-3.0-or-later

package filecheck

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

func discoverFilesOrDirs(includePaths []string, fn func(absPath string, fi os.FileInfo) bool) []string {
	var paths []string

	for _, path := range includePaths {
		if !hasMeta(path) {
			paths = append(paths, path)
			continue
		}

		ps, _ := filepath.Glob(path)
		for _, path := range ps {
			if fi, err := os.Lstat(path); err == nil && fn(path, fi) {
				paths = append(paths, path)
			}
		}

	}

	slices.Sort(paths)
	paths = slices.Compact(paths)

	return paths
}

func hasMeta(path string) bool {
	magicChars := `*?[`
	if runtime.GOOS != "windows" {
		magicChars = `*?[\`
	}
	return strings.ContainsAny(path, magicChars)
}
