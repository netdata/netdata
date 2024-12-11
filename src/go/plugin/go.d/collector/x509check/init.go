// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"errors"
)

func (c *Collector) validateConfig() error {
	if c.Source == "" {
		return errors.New("source is not set")
	}
	return nil
}

func (c *Collector) initProvider() (provider, error) {
	return newProvider(c.Config)
}
