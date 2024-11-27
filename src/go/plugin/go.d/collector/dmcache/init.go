// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package dmcache

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
)

func (c *Collector) initDmsetupCLI() (dmsetupCli, error) {
	ndsudoPath := filepath.Join(executable.Directory, "ndsudo")
	if _, err := os.Stat(ndsudoPath); err != nil {
		return nil, fmt.Errorf("ndsudo executable not found: %v", err)

	}

	dmsetup := newDmsetupExec(ndsudoPath, c.Timeout.Duration(), c.Logger)

	return dmsetup, nil
}
