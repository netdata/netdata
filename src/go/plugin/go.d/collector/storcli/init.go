// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package storcli

func (c *Collector) initStorCliExec() (storCli, error) {
	storExec := newStorCliExec(c.Timeout.Duration(), c.Logger)

	return storExec, nil
}
