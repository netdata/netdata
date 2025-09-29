// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/vnodes"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/ping"
)

type (
	Config struct {
		UpdateEvery int    `yaml:"update_every,omitempty" json:"update_every"`
		Hostname    string `yaml:"hostname" json:"hostname"`

		Community string        `yaml:"community,omitempty" json:"community"`
		User      UserConfig    `yaml:"user,omitempty" json:"user"`
		Options   OptionsConfig `yaml:"options,omitempty" json:"options"`

		CreateVnode              bool               `yaml:"create_vnode,omitempty" json:"create_vnode"`
		VnodeDeviceDownThreshold int                `yaml:"vnode_device_down_threshold,omitempty" json:"vnode_device_down_threshold"`
		Vnode                    vnodes.VirtualNode `yaml:"vnode,omitempty" json:"vnode"`

		ManualProfiles []string `yaml:"manual_profiles,omitempty" json:"manual_profiles"`

		PingOnly bool       `yaml:"ping_only,omitempty" json:"ping_only"`
		Ping     PingConfig `yaml:"ping,omitempty" json:"ping"`
	}

	PingConfig struct {
		Enabled           bool `yaml:"enabled" json:"enabled"`
		ping.ProberConfig `yaml:",inline" json:",inline"`
	}

	UserConfig struct {
		Name          string `yaml:"name,omitempty" json:"name"`
		SecurityLevel string `yaml:"level,omitempty" json:"level"`
		AuthProto     string `yaml:"auth_proto,omitempty" json:"auth_proto"`
		AuthKey       string `yaml:"auth_key,omitempty" json:"auth_key"`
		PrivProto     string `yaml:"priv_proto,omitempty" json:"priv_proto"`
		PrivKey       string `yaml:"priv_key,omitempty" json:"priv_key"`
	}
	OptionsConfig struct {
		Port           int    `yaml:"port,omitempty" json:"port"`
		Retries        int    `yaml:"retries,omitempty" json:"retries"`
		Timeout        int    `yaml:"timeout,omitempty" json:"timeout"`
		Version        string `yaml:"version,omitempty" json:"version"`
		MaxOIDs        int    `yaml:"max_request_size,omitempty" json:"max_request_size"`
		MaxRepetitions int    `yaml:"max_repetitions,omitempty" json:"max_repetitions"`
	}
)
