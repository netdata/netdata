// SPDX-License-Identifier: GPL-3.0-or-later

package jetson

import "errors"

const defaultUpdateEvery = 1

type Config struct {
	UpdateEvery int `yaml:"update_every,omitempty" json:"update_every"`
}

func (c Config) validate() error {
	if c.UpdateEvery < 1 {
		return errors.New("update_every must be at least 1 second")
	}
	return nil
}
