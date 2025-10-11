// SPDX-License-Identifier: GPL-3.0-or-later

package snmpsd

import (
	"fmt"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/iprange"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

type (
	Config struct {
		Source string `yaml:"-"`

		// RescanInterval defines how often to scan the networks for devices (default: 30m)
		RescanInterval *confopt.Duration `yaml:"rescan_interval"`
		// Timeout defines the maximum time to wait for SNMP device responses (default: 1s)
		Timeout confopt.Duration `yaml:"timeout"`
		// DeviceCacheTTL defines how long to trust cached discovery results before requiring a new probe (default: 12h)
		DeviceCacheTTL *confopt.Duration `yaml:"device_cache_ttl"`
		// ParallelScansPerNetwork defines how many IPs to scan concurrently within each subnet (default: 32)
		ParallelScansPerNetwork int `yaml:"parallel_scans_per_network"`
		// Credentials define the SNMP credentials used for authentication
		Credentials []CredentialConfig `yaml:"credentials"`
		// Networks defines the subnets to scan and which credentials to use
		Networks []NetworkConfig `yaml:"networks"`
	}

	NetworkConfig struct {
		// Subnet is the IP range to scan, supporting various formats
		// https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/pkg/iprange#supported-formats
		Subnet string `yaml:"subnet"`
		// Credential is the name of a credential from the Credentials list
		Credential string `yaml:"credential"`
	}
	CredentialConfig struct {
		// Name is the identifier for this credential set, used in Network.Credential
		Name string `yaml:"name"`
		// Version must be one of: "1", "2c", or "3"
		Version string `yaml:"version"`
		// Community is the SNMP community string (used in v1 and v2c)
		Community string `yaml:"community"`
		// UserName is the SNMPv3 username
		UserName string `yaml:"username"`
		// SecurityLevel must be one of: "noAuthNoPriv", "authNoPriv", or "authPriv" (for SNMPv3)
		SecurityLevel string `yaml:"security_level"`
		// AuthProtocol must be one of: "md5", "sha", "sha224", "sha256", "sha384", "sha512" (for SNMPv3)
		AuthProtocol string `yaml:"auth_protocol"`
		// AuthPassphrase is the authentication passphrase (for SNMPv3)
		AuthPassphrase string `yaml:"auth_password"`
		// PrivacyProtocol must be one of: "des", "aes", "aes192", "aes256", "aes192C", "aes256C" (for SNMPv3)
		PrivacyProtocol string `yaml:"priv_protocol"`
		// PrivacyPassphrase is the privacy passphrase (for SNMPv3)
		PrivacyPassphrase string `yaml:"priv_password"`
	}
)

func (c *Config) validateAndParse() ([]subnet, error) {
	if len(c.Credentials) == 0 {
		return nil, fmt.Errorf("no credentials provided")
	}
	if len(c.Networks) == 0 {
		return nil, fmt.Errorf("no networks provided")
	}

	credentials := make(map[string]CredentialConfig)

	for i, cr := range c.Credentials {
		if cr.Name == "" {
			return nil, fmt.Errorf("no name provided for credential %d", i)
		}
		if _, ok := credentials[cr.Name]; ok {
			return nil, fmt.Errorf("duplicate credential name: %s", cr.Name)
		}
		credentials[cr.Name] = c.Credentials[i]
	}

	networks := make(map[string]bool)

	var subnets []subnet

	for i, n := range c.Networks {
		if n.Subnet == "" {
			return nil, fmt.Errorf("no subnet provided for network %d", i)
		}
		if n.Credential == "" {
			return nil, fmt.Errorf("no credential provided for network %s", n.Subnet)
		}
		if _, ok := credentials[n.Credential]; !ok {
			return nil, fmt.Errorf("no credential provided for network %s", n.Subnet)
		}

		r, err := iprange.ParseRange(n.Subnet)
		if err != nil {
			return nil, fmt.Errorf("invalid subnet range '%s': %v", n.Subnet, err)
		}

		// Limit subnet size to /23 or smaller (512 IPs max per subnet)
		// This prevents accidental scanning of excessively large networks.
		if s := r.Size().Int64(); s > 512 {
			return nil, fmt.Errorf("subnet '%s' exceeds maximum size of /23 (512 IPs, got %d IPs)", n.Subnet, s)
		}

		sub := subnet{
			str:        n.Subnet,
			ips:        r,
			credential: credentials[n.Credential],
		}

		if networks[subKey(sub)] {
			return nil, fmt.Errorf("duplicate subnet '%s'", subKey(sub))
		}
		networks[subKey(sub)] = true

		subnets = append(subnets, sub)
	}

	return subnets, nil
}

func setCredential(client gosnmp.Handler, cred CredentialConfig) {
	switch snmputils.ParseSNMPVersion(cred.Version) {
	case gosnmp.Version1:
		client.SetVersion(gosnmp.Version1)
		client.SetCommunity(cred.Community)
	case gosnmp.Version2c:
		client.SetVersion(gosnmp.Version2c)
		client.SetCommunity(cred.Community)
	case gosnmp.Version3:
		client.SetVersion(gosnmp.Version3)
		client.SetSecurityModel(gosnmp.UserSecurityModel)
		client.SetMsgFlags(snmputils.ParseSNMPv3SecurityLevel(cred.SecurityLevel))
		client.SetSecurityParameters(&gosnmp.UsmSecurityParameters{
			UserName:                 cred.UserName,
			AuthenticationProtocol:   snmputils.ParseSNMPv3AuthProtocol(cred.AuthProtocol),
			AuthenticationPassphrase: cred.AuthPassphrase,
			PrivacyProtocol:          snmputils.ParseSNMPv3PrivProtocol(cred.PrivacyProtocol),
			PrivacyPassphrase:        cred.PrivacyPassphrase,
		})
	}
}
