// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package jetson

const defaultUpdateEvery = 1

type Config struct {
	UpdateEvery int `yaml:"update_every,omitempty" json:"update_every"`
}
