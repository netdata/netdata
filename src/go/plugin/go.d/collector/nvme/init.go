// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package nvme

func (c *Collector) initNVMeCLIExec() (nvmeCli, error) {
	nvmeExec := &nvmeCLIExec{timeout: c.Timeout.Duration()}

	return nvmeExec, nil
}
