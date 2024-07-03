// SPDX-License-Identifier: GPL-3.0-or-later

package adaptecraid

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
)

func (a *AdaptecRaid) initArcconfCliExec() (arcconfCli, error) {
	ndsudoPath := filepath.Join(executable.Directory, "ndsudo")

	if _, err := os.Stat(ndsudoPath); err != nil {
		return nil, fmt.Errorf("ndsudo executable not found: %v", err)
	}

	arcconfExec := newArcconfCliExec(ndsudoPath, a.Timeout.Duration(), a.Logger)

	return arcconfExec, nil
}
