// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import "github.com/netdata/netdata/go/plugins/pkg/confopt"

const (
	testDiscovererTypeNetListeners = "net_listeners"
	testDiscovererTypeDocker       = "docker"
	testDiscovererTypeK8s          = "k8s"
	testDiscovererTypeSNMP         = "snmp"
)

type testNetListenersConfig struct {
	Interval confopt.LongDuration `json:"interval,omitempty" yaml:"interval,omitempty"`
	Timeout  confopt.Duration     `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

type testDockerConfig struct {
	Address string           `json:"address,omitempty" yaml:"address,omitempty"`
	Timeout confopt.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

type testK8sConfig struct {
	Role       string   `json:"role,omitempty" yaml:"role,omitempty"`
	Namespaces []string `json:"namespaces,omitempty" yaml:"namespaces,omitempty"`
	Selector   struct {
		Label string `json:"label,omitempty" yaml:"label,omitempty"`
	} `json:"selector,omitempty" yaml:"selector,omitempty"`
	Pod struct {
		LocalMode bool `json:"local_mode,omitempty" yaml:"local_mode,omitempty"`
	} `json:"pod,omitempty" yaml:"pod,omitempty"`
}

type testSNMPConfig struct {
	RescanInterval confopt.LongDuration       `json:"rescan_interval,omitempty" yaml:"rescan_interval,omitempty"`
	Timeout        confopt.Duration           `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	DeviceCacheTTL confopt.LongDuration       `json:"device_cache_ttl,omitempty" yaml:"device_cache_ttl,omitempty"`
	Credentials    []testSNMPCredentialConfig `json:"credentials,omitempty" yaml:"credentials,omitempty"`
	Networks       []testSNMPNetworkConfig    `json:"networks,omitempty" yaml:"networks,omitempty"`
}

type testSNMPCredentialConfig struct {
	Name              string `json:"name,omitempty" yaml:"name,omitempty"`
	Version           string `json:"version,omitempty" yaml:"version,omitempty"`
	Community         string `json:"community,omitempty" yaml:"community,omitempty"`
	UserName          string `json:"username,omitempty" yaml:"username,omitempty"`
	Username          string `json:"-" yaml:"-"`
	SecurityLevel     string `json:"security_level,omitempty" yaml:"security_level,omitempty"`
	AuthProtocol      string `json:"auth_protocol,omitempty" yaml:"auth_protocol,omitempty"`
	AuthPassphrase    string `json:"auth_passphrase,omitempty" yaml:"auth_passphrase,omitempty"`
	AuthKey           string `json:"auth_key,omitempty" yaml:"auth_key,omitempty"`
	PrivacyProtocol   string `json:"privacy_protocol,omitempty" yaml:"privacy_protocol,omitempty"`
	PrivacyPassphrase string `json:"privacy_passphrase,omitempty" yaml:"privacy_passphrase,omitempty"`
	PrivProtocol      string `json:"priv_protocol,omitempty" yaml:"priv_protocol,omitempty"`
	PrivKey           string `json:"priv_key,omitempty" yaml:"priv_key,omitempty"`
}

type testSNMPNetworkConfig struct {
	Subnet     string `json:"subnet,omitempty" yaml:"subnet,omitempty"`
	Credential string `json:"credential,omitempty" yaml:"credential,omitempty"`
}
