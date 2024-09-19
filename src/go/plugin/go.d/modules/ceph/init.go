// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
)

func (c *Ceph) initCephBinary() (cephBinary, error) {

	ndsudoPath := filepath.Join(executable.Directory, "ndsudo")

	if _, err := os.Stat(ndsudoPath); err != nil {
		return nil, fmt.Errorf("ndsudo executable not found: %c", err)

	}

	ceph := newCephExecBinary(ndsudoPath, c.Config, c.Logger)

	return ceph, nil
}
