// SPDX-License-Identifier: GPL-3.0-or-later

package freeradius

import (
	"errors"
)

func (c *Collector) validateConfig() error {
	if c.Address == "" {
		return errors.New("address not set")
	}
	if c.Port == 0 {
		return errors.New("port not set")
	}
	if c.Secret == "" {
		return errors.New("secret not set")
	}
	return nil
}
