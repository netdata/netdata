// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

type EndpointConfig struct {
	Protocol string `yaml:"protocol" json:"protocol"`
	Address  string `yaml:"address" json:"address"`
	Port     int    `yaml:"port" json:"port"`
}

type Config struct {
	Vnode       string              `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery int                 `yaml:"update_every,omitempty" json:"update_every"`
	Listen      ListenConfig        `yaml:"listen" json:"listen"`
	Versions    []string            `yaml:"versions,omitempty" json:"versions"`
	Communities []string            `yaml:"communities,omitempty" json:"communities"`
	Retention   jsonRetentionConfig `yaml:"retention,omitempty" json:"retention"`
}

type ListenConfig struct {
	Endpoints []EndpointConfig `yaml:"endpoints" json:"endpoints"`
}
