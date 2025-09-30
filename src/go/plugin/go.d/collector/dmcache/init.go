// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package dmcache

func (c *Collector) initDmsetupCLI() (dmsetupCli, error) {
	dmsetup := newDmsetupExec(c.Timeout.Duration(), c.Logger)

	return dmsetup, nil
}
