// SPDX-License-Identifier: GPL-3.0-or-later

package dmcache

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/go.d.plugin/agent/executable"
)

func (c *DmCache) initDmsetupCLI() (dmsetupCLI, error) {
	ndsudoPath := filepath.Join(executable.Directory, "ndsudo")
	if _, err := os.Stat(ndsudoPath); err != nil {
		return nil, fmt.Errorf("ndsudo executable not found: %v", err)

	}

	dmsetup := newDmsetupExec(ndsudoPath, c.Timeout.Duration(), c.Logger)

	return dmsetup, nil
}
