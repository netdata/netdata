// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

package nvme

func (c *Collector) initNVMeCLIExec() (nvmeCli, error) {
	return c.initNdsudoNvmeCliExec()
}
