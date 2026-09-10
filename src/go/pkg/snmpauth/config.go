// SPDX-License-Identifier: GPL-3.0-or-later

package snmpauth

import (
	"fmt"

	"github.com/gosnmp/gosnmp"
)

// Config is the authentication contract for new SNMP consumers. Legacy public
// collector and discovery formats retain their own compatibility rules.
type Config struct {
	Version      string     `yaml:"version,omitempty" json:"version,omitempty"`
	Credentials  *Community `yaml:"credentials,omitempty" json:"credentials,omitempty"`
	Credentials3 *USM       `yaml:"credentials3,omitempty" json:"credentials3,omitempty"`
}
type Community struct {
	Community string `yaml:"community" json:"community"`
}
type USM struct {
	Username      string `yaml:"username" json:"username"`
	SecurityLevel string `yaml:"security_level,omitempty" json:"security_level,omitempty"`
	AuthProtocol  string `yaml:"auth_protocol,omitempty" json:"auth_protocol,omitempty"`
	AuthPassword  string `yaml:"auth_password,omitempty" json:"auth_password,omitempty"`
	PrivProtocol  string `yaml:"priv_protocol,omitempty" json:"priv_protocol,omitempty"`
	PrivPassword  string `yaml:"priv_password,omitempty" json:"priv_password,omitempty"`
	ContextName   string `yaml:"context_name,omitempty" json:"context_name,omitempty"`
}

func (c Config) Copy() Config {
	if c.Credentials != nil {
		v := *c.Credentials
		c.Credentials = &v
	}
	if c.Credentials3 != nil {
		v := *c.Credentials3
		c.Credentials3 = &v
	}
	return c
}

// Normalized returns owned credentials for the selected version and security level.
// Forms can retain hidden fields when an operator switches a nested section.
func (c Config) Normalized() Config {
	c = c.Copy()
	switch c.Version {
	case "", "1", "2c":
		c.Credentials3 = nil
	case "3":
		c.Credentials = nil
		if u := c.Credentials3; u != nil {
			switch u.SecurityLevel {
			case "noAuthNoPriv":
				u.AuthProtocol, u.AuthPassword = "", ""
				u.PrivProtocol, u.PrivPassword = "", ""
			case "authNoPriv":
				u.PrivProtocol, u.PrivPassword = "", ""
			}
		}
	}
	return c
}
func (c Config) ContextName() string {
	if c.Version == "3" && c.Credentials3 != nil {
		return c.Credentials3.ContextName
	}
	return ""
}
func (c Config) Validate() error { return c.Apply(&gosnmp.GoSNMP{}) }

// Apply validates before installing credentials. Errors name fields, never values.
func (c Config) Apply(client *gosnmp.GoSNMP) error {
	c = c.Normalized()
	switch c.Version {
	case "", "2c", "1":
		if c.Credentials == nil || c.Credentials.Community == "" {
			return fmt.Errorf("credentials.community is required")
		}
		client.Version = gosnmp.Version2c
		if c.Version == "1" {
			client.Version = gosnmp.Version1
		}
		client.Community = c.Credentials.Community
		return nil
	case "3":
	default:
		return fmt.Errorf("version must be 1, 2c or 3")
	}
	if c.Credentials3 == nil || c.Credentials3.Username == "" {
		return fmt.Errorf("credentials3.username is required")
	}
	u := *c.Credentials3
	if u.SecurityLevel == "" {
		u.SecurityLevel = "authPriv"
	}
	flags, ok := map[string]gosnmp.SnmpV3MsgFlags{"noAuthNoPriv": gosnmp.NoAuthNoPriv, "authNoPriv": gosnmp.AuthNoPriv, "authPriv": gosnmp.AuthPriv}[u.SecurityLevel]
	if !ok {
		return fmt.Errorf("invalid credentials3.security_level")
	}
	auth, priv := gosnmp.NoAuth, gosnmp.NoPriv
	if flags != gosnmp.NoAuthNoPriv {
		if u.AuthProtocol == "" {
			u.AuthProtocol = "sha512"
		}
		auth, ok = map[string]gosnmp.SnmpV3AuthProtocol{"md5": gosnmp.MD5, "sha": gosnmp.SHA, "sha224": gosnmp.SHA224, "sha256": gosnmp.SHA256, "sha384": gosnmp.SHA384, "sha512": gosnmp.SHA512}[u.AuthProtocol]
		if !ok {
			return fmt.Errorf("invalid credentials3.auth_protocol")
		}
		if len(u.AuthPassword) < 8 {
			return fmt.Errorf("credentials3.auth_password must contain at least 8 bytes")
		}
	}
	if flags == gosnmp.AuthPriv {
		if u.PrivProtocol == "" {
			u.PrivProtocol = "aes192c"
		}
		priv, ok = map[string]gosnmp.SnmpV3PrivProtocol{"des": gosnmp.DES, "aes": gosnmp.AES, "aes192": gosnmp.AES192, "aes256": gosnmp.AES256, "aes192c": gosnmp.AES192C, "aes256c": gosnmp.AES256C}[u.PrivProtocol]
		if !ok {
			return fmt.Errorf("invalid credentials3.priv_protocol")
		}
		if len(u.PrivPassword) < 8 {
			return fmt.Errorf("credentials3.priv_password must contain at least 8 bytes")
		}
	}
	client.Version = gosnmp.Version3
	client.MsgFlags = flags
	client.SecurityModel = gosnmp.UserSecurityModel
	client.ContextName = u.ContextName
	client.SecurityParameters = &gosnmp.UsmSecurityParameters{UserName: u.Username, AuthenticationProtocol: auth, AuthenticationPassphrase: u.AuthPassword, PrivacyProtocol: priv, PrivacyPassphrase: u.PrivPassword}
	return nil
}
