// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly || darwin

package nsd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
)

func (n *Nsd) initNsdControlExec() (nsdControlBinary, error) {
	ndsudoPath := filepath.Join(executable.Directory, "ndsudo")
	if _, err := os.Stat(ndsudoPath); err != nil {
		return nil, fmt.Errorf("ndsudo executable not found: %v", err)

	}

	nsdControl := newNsdControlExec(ndsudoPath, n.Timeout.Duration(), n.Logger)

	return nsdControl, nil
}
