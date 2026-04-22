// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import "github.com/netdata/netdata/go/plugins/pkg/confopt"

// Config for the snmp_topology module.
// This module has a single global job — device list comes from the SNMP device registry.
type Config struct {
	UpdateEvery  int                  `yaml:"update_every,omitempty" json:"update_every"`
	RefreshEvery confopt.LongDuration `yaml:"refresh_every,omitempty" json:"refresh_every,omitempty"`
}
