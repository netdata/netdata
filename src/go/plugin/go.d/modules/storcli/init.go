// SPDX-License-Identifier: GPL-3.0-or-later

package storcli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
)

func (s *StorCli) initStorCliExec() (storCli, error) {
	ndsudoPath := filepath.Join(executable.Directory, "ndsudo")

	if _, err := os.Stat(ndsudoPath); err != nil {
		return nil, fmt.Errorf("ndsudo executable not found: %v", err)
	}

	storExec := newStorCliExec(ndsudoPath, s.Timeout.Duration(), s.Logger)

	return storExec, nil
}
