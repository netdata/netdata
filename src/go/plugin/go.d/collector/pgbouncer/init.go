// SPDX-License-Identifier: GPL-3.0-or-later

package pgbouncer

import "errors"

func (c *Collector) validateConfig() error {
	if c.DSN == "" {
		return errors.New("DSN not set")
	}
	return nil
}
