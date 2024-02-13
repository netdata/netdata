// SPDX-License-Identifier: GPL-3.0-or-later

package filecheck

import (
	"regexp"
	"runtime"
	"strings"
)

func (fc *Filecheck) collect() (map[string]int64, error) {
	ms := make(map[string]int64)

	fc.collectFiles(ms)
	fc.collectDirs(ms)

	return ms, nil
}

func hasMeta(path string) bool {
	magicChars := `*?[`
	if runtime.GOOS != "windows" {
		magicChars = `*?[\`
	}
	return strings.ContainsAny(path, magicChars)
}

func removeDuplicates(s []string) []string {
	set := make(map[string]bool, len(s))
	uniq := s[:0]
	for _, v := range s {
		if !set[v] {
			set[v] = true
			uniq = append(uniq, v)
		}
	}
	return uniq
}

var reSpace = regexp.MustCompile(`\s`)
