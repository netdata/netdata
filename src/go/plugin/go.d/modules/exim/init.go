// SPDX-License-Identifier: GPL-3.0-or-later

package exim

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
)

func (e *Exim) initEximExec() (eximBinary, error) {
	ndsudoPath := filepath.Join(executable.Directory, "ndsudo")
	if _, err := os.Stat(ndsudoPath); err != nil {
		return nil, fmt.Errorf("ndsudo executable not found: %v", err)

	}

	exim := newEximExec(ndsudoPath, e.Timeout.Duration(), e.Logger)

	return exim, nil
}
