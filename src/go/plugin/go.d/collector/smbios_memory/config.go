// SPDX-License-Identifier: GPL-3.0-or-later

package smbios_memory

import "errors"

const defaultUpdateEvery = 60

type Config struct {
	// Vnode is recognized only to reject moving the host's inventory to a virtual node.
	Vnode       string `yaml:"vnode,omitempty" json:"vnode,omitempty"`
	UpdateEvery int    `yaml:"update_every,omitempty" json:"update_every"`
}

func (c Config) validate() error {
	if c.Vnode != "" {
		return errors.New("smbios_memory monitors the local host and does not support vnode")
	}
	if c.UpdateEvery <= 0 {
		return errors.New("update_every must be positive")
	}
	return nil
}
