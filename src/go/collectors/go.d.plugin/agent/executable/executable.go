// SPDX-License-Identifier: GPL-3.0-or-later

package executable

import (
	"os"
	"path/filepath"
	"strings"
)

var (
	Name      string
	Directory string
)

func init() {
	path, err := os.Executable()
	if err != nil || path == "" {
		Name = "go.d"
		return
	}

	_, Name = filepath.Split(path)
	Name = strings.TrimSuffix(Name, ".plugin")

	if strings.HasSuffix(Name, ".test") {
		Name = "test"
	}

	// FIXME: can't use logger because of circular import
	fi, err := os.Lstat(path)
	if err != nil {
		return
	}

	if fi.Mode()&os.ModeSymlink != 0 {
		realPath, err := filepath.EvalSymlinks(path)
		if err != nil {
			return
		}
		Directory = filepath.Dir(realPath)
	} else {
		Directory = filepath.Dir(path)
	}
}
