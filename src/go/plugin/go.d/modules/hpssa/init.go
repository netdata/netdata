// SPDX-License-Identifier: GPL-3.0-or-later

package hpssa

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
)

func (h *Hpssa) initSsacliExec() (ssacli, error) {
	ndsudoPath := filepath.Join(executable.Directory, "ndsudo")

	if _, err := os.Stat(ndsudoPath); err != nil {
		return nil, fmt.Errorf("ndsudo executable not found: %v", err)
	}

	ssacliExec := newSsacliExec(ndsudoPath, h.Timeout.Duration(), h.Logger)

	return ssacliExec, nil
}
