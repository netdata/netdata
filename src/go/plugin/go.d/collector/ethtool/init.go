// SPDX-License-Identifier: GPL-3.0-or-later

package ethtool

import (
	"errors"
)

func (c *Collector) validateConfig() error {
	if c.OpticInterfaces == "" {
		return errors.New("no optic interfaces specified")
	}
	return nil
}

func (c *Collector) initEthtoolCli() (ethtoolCli, error) {
	et := newEthtoolExec(c.Timeout.Duration(), c.Logger)

	return et, nil
}
