// SPDX-License-Identifier: GPL-3.0-or-later

package snmpsd

import (
	"fmt"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/iprange"
)

type (
	Config struct {
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
		AuthPassphrase string `yaml:"auth_passphrase"`
		// PrivacyProtocol must be one of: "des", "aes", "aes192", "aes256", "aes192C", "aes256C" (for SNMPv3)
		PrivacyProtocol string `yaml:"privacy_protocol"`
		// PrivacyPassphrase is the privacy passphrase (for SNMPv3)
		PrivacyPassphrase string `yaml:"privacy_passphrase"`
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
	switch parseSNMPVersion(cred) {
	case gosnmp.Version1:
		client.SetVersion(gosnmp.Version1)
		client.SetCommunity(cred.Community)
	case gosnmp.Version2c:
		client.SetVersion(gosnmp.Version2c)
		client.SetCommunity(cred.Community)
	case gosnmp.Version3:
		client.SetVersion(gosnmp.Version3)
		client.SetSecurityModel(gosnmp.UserSecurityModel)
		client.SetMsgFlags(parseSNMPv3SecurityLevel(cred))
		client.SetSecurityParameters(&gosnmp.UsmSecurityParameters{
			UserName:                 cred.UserName,
			AuthenticationProtocol:   parseSNMPv3AuthProtocol(cred),
			AuthenticationPassphrase: cred.AuthPassphrase,
			PrivacyProtocol:          parseSNMPv3PrivProtocol(cred),
			PrivacyPassphrase:        cred.PrivacyPassphrase,
		})
	}
}

func parseSNMPVersion(cred CredentialConfig) gosnmp.SnmpVersion {
	switch cred.Version {
	case "0", "1":
		return gosnmp.Version1
	case "2", "2c", "":
		return gosnmp.Version2c
	case "3":
		return gosnmp.Version3
	default:
		return gosnmp.Version2c
	}
}

func parseSNMPv3SecurityLevel(cred CredentialConfig) gosnmp.SnmpV3MsgFlags {
	switch cred.SecurityLevel {
	case "1", "none", "noAuthNoPriv", "":
		return gosnmp.NoAuthNoPriv
	case "2", "authNoPriv":
		return gosnmp.AuthNoPriv
	case "3", "authPriv":
		return gosnmp.AuthPriv
	default:
		return gosnmp.NoAuthNoPriv
	}
}

func parseSNMPv3AuthProtocol(cred CredentialConfig) gosnmp.SnmpV3AuthProtocol {
	switch cred.AuthProtocol {
	case "1", "none", "noAuth", "":
		return gosnmp.NoAuth
	case "2", "md5", "MD5":
		return gosnmp.MD5
	case "3", "sha", "SHA":
		return gosnmp.SHA
	case "4", "sha224", "SHA224":
		return gosnmp.SHA224
	case "5", "sha256", "SHA256":
		return gosnmp.SHA256
	case "6", "sha384", "SHA384":
		return gosnmp.SHA384
	case "7", "sha512", "SHA512":
		return gosnmp.SHA512
	default:
		return gosnmp.NoAuth
	}
}

func parseSNMPv3PrivProtocol(cred CredentialConfig) gosnmp.SnmpV3PrivProtocol {
	switch cred.PrivacyProtocol {
	case "1", "none", "noPriv", "":
		return gosnmp.NoPriv
	case "2", "des", "DES":
		return gosnmp.DES
	case "3", "aes", "AES":
		return gosnmp.AES
	case "4", "aes192", "AES192":
		return gosnmp.AES192
	case "5", "aes256", "AES256":
		return gosnmp.AES256
	case "6", "aes192c", "AES192C":
		return gosnmp.AES192C
	case "7", "aes256c", "AES256C":
		return gosnmp.AES256C
	default:
		return gosnmp.NoPriv
	}
}
