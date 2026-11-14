// SPDX-License-Identifier: GPL-3.0-or-later

package snmputils

import (
	"fmt"
	"strings"

	"github.com/netdata/gosnmp"
)

func ParseSNMPVersion(version string) gosnmp.Version {
	switch version {
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

func ParseSNMPv3SecurityLevel(level string) gosnmp.SecurityLevel {
	switch level {
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

// SNMPv3SecurityLevel returns the USM security level the given protocols provide. Privacy without
// authentication is not a valid USM level, so it yields NoAuthNoPriv.
func SNMPv3SecurityLevel(auth gosnmp.AuthProtocol, priv gosnmp.PrivProtocol) gosnmp.SecurityLevel {
	switch {
	case auth == 0 || auth == gosnmp.NoAuth:
		return gosnmp.NoAuthNoPriv
	case priv == 0 || priv == gosnmp.NoPriv:
		return gosnmp.AuthNoPriv
	default:
		return gosnmp.AuthPriv
	}
}

func ParseSNMPv3AuthProtocol(protocol string) gosnmp.AuthProtocol {
	switch protocol {
	case "1", "none", "noAuth", "":
		return gosnmp.NoAuth
	case "2", "md5":
		return gosnmp.MD5
	case "3", "sha":
		return gosnmp.SHA
	case "4", "sha224":
		return gosnmp.SHA224
	case "5", "sha256":
		return gosnmp.SHA256
	case "6", "sha384":
		return gosnmp.SHA384
	case "7", "sha512":
		return gosnmp.SHA512
	default:
		return gosnmp.NoAuth
	}
}

func ParseSNMPv3PrivProtocol(protocol string) gosnmp.PrivProtocol {
	switch protocol {
	case "1", "none", "noPriv", "":
		return gosnmp.NoPriv
	case "2", "des":
		return gosnmp.DES
	case "3", "aes":
		return gosnmp.AES
	case "4", "aes192":
		return gosnmp.AES192
	case "5", "aes256":
		return gosnmp.AES256
	case "6", "aes192c":
		return gosnmp.AES192C
	case "7", "aes256c":
		return gosnmp.AES256C
	default:
		return gosnmp.NoPriv
	}
}

// SnmpClientConnInfo describes a client configuration for logging. It omits
// secrets: the community and the USM passphrases.
func SnmpClientConnInfo(cfg gosnmp.Config) string {
	var info strings.Builder
	fmt.Fprintf(&info, "hostname='%s',port='%d',snmp_version='%s'", cfg.Target, cfg.Port, cfg.Version)
	if cfg.Version != gosnmp.Version3 {
		return info.String()
	}

	fmt.Fprintf(&info, ",security_level='%s'", cfg.SecurityLevel)
	fmt.Fprintf(&info, ",username='%s',auth_protocol='%s',priv_protocol='%s'",
		cfg.User.Name, cfg.User.AuthProtocol, cfg.User.PrivProtocol)
	if cfg.ContextName != "" {
		fmt.Fprintf(&info, ",context_name='%s'", cfg.ContextName)
	}
	return info.String()
}
