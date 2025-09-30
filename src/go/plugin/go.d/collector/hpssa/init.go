// SPDX-License-Identifier: GPL-3.0-or-later

package hpssa

func (c *Collector) initSsacliBinary() (ssacliBinary, error) {
	ssacliExec := newSsacliExec(c.Timeout.Duration(), c.Logger)

	return ssacliExec, nil
}
