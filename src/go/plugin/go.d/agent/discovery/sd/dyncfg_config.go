// SPDX-License-Identifier: GPL-3.0-or-later

package sd

// Dyncfg config structs for each discoverer type.
// These are the flattened representations used by the UI, with proper json/yaml tags.

type DyncfgNetListenersConfig struct {
	Name     string `json:"name" yaml:"name"`
	Disabled bool   `json:"disabled,omitempty" yaml:"disabled,omitempty"`

	Tags     string `json:"tags,omitempty" yaml:"tags,omitempty"`
	Interval string `json:"interval,omitempty" yaml:"interval,omitempty"`
	Timeout  string `json:"timeout,omitempty" yaml:"timeout,omitempty"`

	Services []DyncfgServiceRule `json:"services,omitempty" yaml:"services,omitempty"`
}

type DyncfgDockerConfig struct {
	Name     string `json:"name" yaml:"name"`
	Disabled bool   `json:"disabled,omitempty" yaml:"disabled,omitempty"`

	Tags    string `json:"tags,omitempty" yaml:"tags,omitempty"`
	Address string `json:"address,omitempty" yaml:"address,omitempty"`
	Timeout string `json:"timeout,omitempty" yaml:"timeout,omitempty"`

	Services []DyncfgServiceRule `json:"services,omitempty" yaml:"services,omitempty"`
}

type DyncfgK8sConfig struct {
	Name     string `json:"name" yaml:"name"`
	Disabled bool   `json:"disabled,omitempty" yaml:"disabled,omitempty"`

	Role       string               `json:"role,omitempty" yaml:"role,omitempty"`
	Tags       string               `json:"tags,omitempty" yaml:"tags,omitempty"`
	Namespaces []string             `json:"namespaces,omitempty" yaml:"namespaces,omitempty"`
	Selector   *DyncfgK8sSelector   `json:"selector,omitempty" yaml:"selector,omitempty"`
	Pod        *DyncfgK8sPodOptions `json:"pod,omitempty" yaml:"pod,omitempty"`

	Services []DyncfgServiceRule `json:"services,omitempty" yaml:"services,omitempty"`
}

type DyncfgK8sSelector struct {
	Label string `json:"label,omitempty" yaml:"label,omitempty"`
	Field string `json:"field,omitempty" yaml:"field,omitempty"`
}

type DyncfgK8sPodOptions struct {
	LocalMode bool `json:"local_mode,omitempty" yaml:"local_mode,omitempty"`
}

type DyncfgSNMPConfig struct {
	Name     string `json:"name" yaml:"name"`
	Disabled bool   `json:"disabled,omitempty" yaml:"disabled,omitempty"`

	RescanInterval          string `json:"rescan_interval,omitempty" yaml:"rescan_interval,omitempty"`
	Timeout                 string `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	DeviceCacheTTL          string `json:"device_cache_ttl,omitempty" yaml:"device_cache_ttl,omitempty"`
	ParallelScansPerNetwork int    `json:"parallel_scans_per_network,omitempty" yaml:"parallel_scans_per_network,omitempty"`

	Credentials []DyncfgSNMPCredential `json:"credentials,omitempty" yaml:"credentials,omitempty"`
	Networks    []DyncfgSNMPNetwork    `json:"networks,omitempty" yaml:"networks,omitempty"`

	Services []DyncfgServiceRule `json:"services,omitempty" yaml:"services,omitempty"`
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
