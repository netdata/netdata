// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package fail2ban

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
)

func (f *Fail2Ban) initFail2banClientCliExec() (fail2banClientCli, error) {
	ndsudoPath := filepath.Join(executable.Directory, "ndsudo")
	if _, err := os.Stat(ndsudoPath); err != nil {
		return nil, fmt.Errorf("ndsudo executable not found: %v", err)

	}

	f2bClientExec := newFail2BanClientCliExec(ndsudoPath, f.Timeout.Duration(), f.Logger)

	return f2bClientExec, nil
}
