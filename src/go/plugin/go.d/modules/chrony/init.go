// SPDX-License-Identifier: GPL-3.0-or-later

package chrony

import (
	"errors"
)

func (c *Chrony) validateConfig() error {
	if c.Address == "" {
		return errors.New("empty 'address'")
	}
	return nil
}
