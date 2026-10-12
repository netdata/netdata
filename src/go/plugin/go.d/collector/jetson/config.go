// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package jetson

const defaultUpdateEvery = 1

type Config struct {
	Name            string `yaml:"name,omitempty" json:"name,omitempty"`
	UpdateEvery     int    `yaml:"update_every,omitempty" json:"update_every"`
	AllowPrivileged bool   `yaml:"allow_privileged" json:"allow_privileged"`
}
