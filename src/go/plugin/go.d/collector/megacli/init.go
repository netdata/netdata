// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package megacli

func (c *Collector) initMegaCliExec() (megaCli, error) {
	megaExec := newMegaCliExec(c.Timeout.Duration(), c.Logger)

	return megaExec, nil
}
