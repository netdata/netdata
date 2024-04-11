// SPDX-License-Identifier: GPL-3.0-or-later

package nvme

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/go.d.plugin/agent/executable"
)

func (n *NVMe) initNVMeCLIExec() (nvmeCLI, error) {
	ndsudoPath := filepath.Join(executable.Directory, "ndsudo")

	if _, err := os.Stat(ndsudoPath); err != nil {
		return nil, fmt.Errorf("ndsudo executable not found: %v", err)
	}

	nvmeExec := &nvmeCLIExec{
		ndsudoPath: ndsudoPath,
		timeout:    n.Timeout.Duration(),
	}

	return nvmeExec, nil
}
