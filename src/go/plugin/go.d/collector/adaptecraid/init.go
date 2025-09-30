// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package adaptecraid

func (c *Collector) initArcconfCliExec() (arcconfCli, error) {
	arcconfExec := newArcconfCliExec(c.Timeout.Duration(), c.Logger)
	return arcconfExec, nil
}
