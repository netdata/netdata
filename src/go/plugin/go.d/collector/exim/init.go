// SPDX-License-Identifier: GPL-3.0-or-later

package exim

func (c *Collector) initEximExec() (eximBinary, error) {
	exim := newEximExec(c.Timeout.Duration(), c.Logger)

	return exim, nil
}
