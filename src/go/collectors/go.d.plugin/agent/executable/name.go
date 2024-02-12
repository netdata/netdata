// SPDX-License-Identifier: GPL-3.0-or-later

package executable

import (
	"os"
	"path/filepath"
	"strings"
)

var Name string

func init() {
	s, err := os.Executable()
	if err != nil || s == "" || strings.HasSuffix(s, ".test") {
		Name = "go.d"
		return
	}

	_, Name = filepath.Split(s)
	Name = strings.TrimSuffix(Name, ".plugin")
}
