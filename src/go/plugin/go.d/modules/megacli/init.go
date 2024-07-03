// SPDX-License-Identifier: GPL-3.0-or-later

package megacli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
)

func (m *MegaCli) initMegaCliExec() (megaCli, error) {
	ndsudoPath := filepath.Join(executable.Directory, "ndsudo")

	if _, err := os.Stat(ndsudoPath); err != nil {
		return nil, fmt.Errorf("ndsudo executable not found: %v", err)
	}

	megaExec := newMegaCliExec(ndsudoPath, m.Timeout.Duration(), m.Logger)

	return megaExec, nil
}
