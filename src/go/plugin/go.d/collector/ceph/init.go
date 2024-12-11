// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"fmt"
)

func (c *Collector) validateConfig() error {
	if c.URL == "" {
		return fmt.Errorf("URL is required but not set")
	}
	if c.Username == "" || c.Password == "" {
		return fmt.Errorf("username and password are required but not set")
	}
	return nil
}
