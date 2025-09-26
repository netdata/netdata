// SPDX-License-Identifier: GPL-3.0-or-later

package snmputils

import (
	"fmt"
	"strings"

	"github.com/gosnmp/gosnmp"
)

func ParseSNMPVersion(version string) gosnmp.SnmpVersion {
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

func ParseSNMPv3SecurityLevel(level string) gosnmp.SnmpV3MsgFlags {
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

func ParseSNMPv3AuthProtocol(protocol string) gosnmp.SnmpV3AuthProtocol {
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

func ParseSNMPv3PrivProtocol(protocol string) gosnmp.SnmpV3PrivProtocol {
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

func SnmpClientConnInfo(c gosnmp.Handler) string {
	var info strings.Builder
	info.WriteString(fmt.Sprintf("hostname='%s',port='%d',snmp_version='%s'", c.Target(), c.Port(), c.Version()))
	switch c.Version() {
	case gosnmp.Version1, gosnmp.Version2c:
		info.WriteString(fmt.Sprintf(",community='%s'", c.Community()))
	case gosnmp.Version3:
		info.WriteString(fmt.Sprintf(",security_level='%d,%s'", c.MsgFlags(), c.SecurityParameters().Description()))
	}
	return info.String()
}
