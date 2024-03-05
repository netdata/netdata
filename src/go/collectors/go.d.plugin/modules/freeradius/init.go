// SPDX-License-Identifier: GPL-3.0-or-later

package freeradius

import (
	"errors"
)

func (f *FreeRADIUS) validateConfig() error {
	if f.Address == "" {
		return errors.New("address not set")
	}
	if f.Port == 0 {
		return errors.New("port not set")
	}
	if f.Secret == "" {
		return errors.New("secret not set")
	}
	return nil
}
