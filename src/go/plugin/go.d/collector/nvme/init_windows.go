// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package nvme

func (c *Collector) initNVMeCLIExec() (nvmeCli, error) {
	var fallback nvmeCli
	if cli, err := c.initDirectNvmeCliExec(); err == nil {
		fallback = cli
	} else {
		c.Debugf("nvme CLI fallback is not available: %v", err)
	}

	return &windowsNvmeCliExec{
		Logger:   c.Logger,
		native:   newWindowsNvmeExec(c.Logger),
		fallback: fallback,
	}, nil
}
