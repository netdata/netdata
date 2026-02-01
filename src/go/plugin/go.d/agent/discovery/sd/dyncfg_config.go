// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import "github.com/netdata/netdata/go/plugins/pkg/confopt"

// Dyncfg config structs for each discoverer type.
// These match the JSON schema format with discoverer wrapper for unified file/dyncfg config format.

// Net Listeners

type DyncfgNetListenersConfig struct {
	Name       string                            `json:"name" yaml:"name"`
	Disabled   bool                              `json:"disabled,omitempty" yaml:"disabled,omitempty"`
	Discoverer DyncfgNetListenersDiscoverer      `json:"discoverer" yaml:"discoverer"`
	Services   []DyncfgServiceRule               `json:"services,omitempty" yaml:"services,omitempty"`
}

type DyncfgNetListenersDiscoverer struct {
	NetListeners DyncfgNetListenersOptions `json:"net_listeners" yaml:"net_listeners"`
}

type DyncfgNetListenersOptions struct {
	Interval confopt.Duration `json:"interval,omitempty" yaml:"interval,omitempty"`
	Timeout  confopt.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// Docker

type DyncfgDockerConfig struct {
	Name       string                   `json:"name" yaml:"name"`
	Disabled   bool                     `json:"disabled,omitempty" yaml:"disabled,omitempty"`
	Discoverer DyncfgDockerDiscoverer   `json:"discoverer" yaml:"discoverer"`
	Services   []DyncfgServiceRule      `json:"services,omitempty" yaml:"services,omitempty"`
}

type DyncfgDockerDiscoverer struct {
	Docker DyncfgDockerOptions `json:"docker" yaml:"docker"`
}

type DyncfgDockerOptions struct {
	Address string           `json:"address,omitempty" yaml:"address,omitempty"`
	Timeout confopt.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// Kubernetes

type DyncfgK8sConfig struct {
	Name       string              `json:"name" yaml:"name"`
	Disabled   bool                `json:"disabled,omitempty" yaml:"disabled,omitempty"`
	Discoverer DyncfgK8sDiscoverer `json:"discoverer" yaml:"discoverer"`
	Services   []DyncfgServiceRule `json:"services,omitempty" yaml:"services,omitempty"`
}

type DyncfgK8sDiscoverer struct {
	K8s []DyncfgK8sOptions `json:"k8s" yaml:"k8s"`
}

type DyncfgK8sOptions struct {
	Role       string               `json:"role,omitempty" yaml:"role,omitempty"`
	Namespaces []string             `json:"namespaces,omitempty" yaml:"namespaces,omitempty"`
	Selector   *DyncfgK8sSelector   `json:"selector,omitempty" yaml:"selector,omitempty"`
	Pod        *DyncfgK8sPodOptions `json:"pod,omitempty" yaml:"pod,omitempty"`
}

type DyncfgK8sSelector struct {
	Label string `json:"label,omitempty" yaml:"label,omitempty"`
	Field string `json:"field,omitempty" yaml:"field,omitempty"`
}

type DyncfgK8sPodOptions struct {
	LocalMode bool `json:"local_mode,omitempty" yaml:"local_mode,omitempty"`
}

// SNMP

type DyncfgSNMPConfig struct {
	Name       string               `json:"name" yaml:"name"`
	Disabled   bool                 `json:"disabled,omitempty" yaml:"disabled,omitempty"`
	Discoverer DyncfgSNMPDiscoverer `json:"discoverer" yaml:"discoverer"`
	Services   []DyncfgServiceRule  `json:"services,omitempty" yaml:"services,omitempty"`
}

type DyncfgSNMPDiscoverer struct {
	SNMP DyncfgSNMPOptions `json:"snmp" yaml:"snmp"`
}

type DyncfgSNMPOptions struct {
	RescanInterval          confopt.Duration `json:"rescan_interval,omitempty" yaml:"rescan_interval,omitempty"`
	Timeout                 confopt.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	DeviceCacheTTL          confopt.Duration `json:"device_cache_ttl,omitempty" yaml:"device_cache_ttl,omitempty"`
	ParallelScansPerNetwork int              `json:"parallel_scans_per_network,omitempty" yaml:"parallel_scans_per_network,omitempty"`

	Credentials []DyncfgSNMPCredential `json:"credentials,omitempty" yaml:"credentials,omitempty"`
	Networks    []DyncfgSNMPNetwork    `json:"networks,omitempty" yaml:"networks,omitempty"`
}

type DyncfgSNMPCredential struct {
	Name          string `json:"name" yaml:"name"`
	Version       string `json:"version" yaml:"version"`
	Community     string `json:"community,omitempty" yaml:"community,omitempty"`
	Username      string `json:"username,omitempty" yaml:"username,omitempty"`
	SecurityLevel string `json:"security_level,omitempty" yaml:"security_level,omitempty"`
	AuthProtocol  string `json:"auth_protocol,omitempty" yaml:"auth_protocol,omitempty"`
	AuthPassword  string `json:"auth_password,omitempty" yaml:"auth_password,omitempty"`
	PrivProtocol  string `json:"priv_protocol,omitempty" yaml:"priv_protocol,omitempty"`
	PrivPassword  string `json:"priv_password,omitempty" yaml:"priv_password,omitempty"`
}

type DyncfgSNMPNetwork struct {
	Subnet     string `json:"subnet" yaml:"subnet"`
	Credential string `json:"credential" yaml:"credential"`
}

type DyncfgServiceRule struct {
	ID             string `json:"id" yaml:"id"`
	Match          string `json:"match" yaml:"match"`
	ConfigTemplate string `json:"config_template,omitempty" yaml:"config_template,omitempty"`
}
