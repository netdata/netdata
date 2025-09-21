// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/vnodes"

type (
	Config struct {
		UpdateEvery              int                `yaml:"update_every,omitempty" json:"update_every"`
		Hostname                 string             `yaml:"hostname" json:"hostname"`
		CreateVnode              bool               `yaml:"create_vnode,omitempty" json:"create_vnode"`
		VnodeDeviceDownThreshold int                `yaml:"vnode_device_down_threshold,omitempty" json:"vnode_device_down_threshold"`
		Vnode                    vnodes.VirtualNode `yaml:"vnode,omitempty" json:"vnode"`
		Community                string             `yaml:"community,omitempty" json:"community"`
		User                     User               `yaml:"user,omitempty" json:"user"`
		Options                  Options            `yaml:"options,omitempty" json:"options"`

		ChartsInput                []ChartConfig          `yaml:"charts,omitempty" json:"charts"`
		NetworkInterfaceFilter     NetworkInterfaceFilter `yaml:"network_interface_filter,omitempty" json:"network_interface_filter"`
		EnableProfiles             bool                   `yaml:"enable_profiles,omitempty" json:"enable_profiles"`
		EnableProfilesTableMetrics bool                   `yaml:"enable_profiles_table_metrics,omitempty" json:"enable_profiles_table_metrics"`
		ManualProfiles             []string               `yaml:"manual_profiles,omitempty" json:"manual_profiles"`
		DisableLegacyCollection    bool                   `yaml:"disable_legacy_collection,omitempty" json:"disable_legacy_collection"`
	}
	NetworkInterfaceFilter struct {
		ByName string `yaml:"by_name,omitempty" json:"by_name"`
		ByType string `yaml:"by_type,omitempty" json:"by_type"`
	}
	User struct {
		Name          string `yaml:"name,omitempty" json:"name"`
		SecurityLevel string `yaml:"level,omitempty" json:"level"`
		AuthProto     string `yaml:"auth_proto,omitempty" json:"auth_proto"`
		AuthKey       string `yaml:"auth_key,omitempty" json:"auth_key"`
		PrivProto     string `yaml:"priv_proto,omitempty" json:"priv_proto"`
		PrivKey       string `yaml:"priv_key,omitempty" json:"priv_key"`
	}
	Options struct {
		Port           int    `yaml:"port,omitempty" json:"port"`
		Retries        int    `yaml:"retries,omitempty" json:"retries"`
		Timeout        int    `yaml:"timeout,omitempty" json:"timeout"`
		Version        string `yaml:"version,omitempty" json:"version"`
		MaxOIDs        int    `yaml:"max_request_size,omitempty" json:"max_request_size"`
		MaxRepetitions int    `yaml:"max_repetitions,omitempty" json:"max_repetitions"`
	}
	ChartConfig struct {
		ID         string            `yaml:"id" json:"id"`
		Title      string            `yaml:"title" json:"title"`
		Units      string            `yaml:"units" json:"units"`
		Family     string            `yaml:"family" json:"family"`
		Type       string            `yaml:"type" json:"type"`
		Priority   int               `yaml:"priority" json:"priority"`
		IndexRange []int             `yaml:"multiply_range,omitempty" json:"multiply_range"`
		Dimensions []DimensionConfig `yaml:"dimensions" json:"dimensions"`
	}
	DimensionConfig struct {
		OID        string `yaml:"oid" json:"oid"`
		Name       string `yaml:"name" json:"name"`
		Algorithm  string `yaml:"algorithm" json:"algorithm"`
		Multiplier int    `yaml:"multiplier" json:"multiplier"`
		Divisor    int    `yaml:"divisor" json:"divisor"`
	}
)
