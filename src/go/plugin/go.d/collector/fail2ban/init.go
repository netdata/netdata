// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package fail2ban

func (c *Collector) initFail2banClientCliExec() (fail2banClientCli, error) {
	f2bClientExec := newFail2BanClientCliExec(c.Timeout.Duration(), c.Logger)

	return f2bClientExec, nil
}
