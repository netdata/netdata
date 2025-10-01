// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly || darwin

package nsd

func (c *Collector) initNsdControlExec() (nsdControlBinary, error) {
	nsdControl := newNsdControlExec(c.Timeout.Duration(), c.Logger)

	return nsdControl, nil
}
