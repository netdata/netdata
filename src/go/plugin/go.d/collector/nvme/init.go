// SPDX-License-Identifier: GPL-3.0-or-later

package nvme

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

func (c *Collector) initNVMeCLIExec() (nvmeCli, error) {
	if runtime.GOOS == "windows" {
		return c.initDirectNvmeCliExec()
	}
	return c.initNdsudoNvmeCliExec()
}

func (c *Collector) initNdsudoNvmeCliExec() (nvmeCli, error) {
	nvmeExec := &ndsudoNvmeCliExec{timeout: c.Timeout.Duration()}
	return nvmeExec, nil
}

func (c *Collector) initDirectNvmeCliExec() (nvmeCli, error) {
	path, err := ndexec.FindBinary(
		[]string{"nvme", "nvme-cli"},
		[]string{
			filepath.Join(os.Getenv("ProgramFiles"), "nvme-cli", "nvme.exe"),
			filepath.Join(os.Getenv("ProgramFiles(x86)"), "nvme-cli", "nvme.exe"),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("nvme: %w", err)
	}

	c.Debugf("found nvme at: %s", path)

	return &directNvmeCliExec{
		Logger:   c.Logger,
		nvmePath: path,
		timeout:  c.Timeout.Duration(),
	}, nil
}
