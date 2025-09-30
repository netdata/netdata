// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || netbsd

package lvm

func (c *Collector) initLVMCLIExec() (lvmCLI, error) {
	lvmExec := newLVMCLIExec(c.Timeout.Duration(), c.Logger)

	return lvmExec, nil
}
