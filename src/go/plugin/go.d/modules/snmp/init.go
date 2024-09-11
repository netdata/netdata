// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/matcher"

	"github.com/gosnmp/gosnmp"
)

func (s *SNMP) validateConfig() error {
	if s.Hostname == "" {
		return errors.New("SNMP hostname is required")
	}
	if s.Vnode.GUID != "" {
		if err := uuid.Validate(s.Vnode.GUID); err != nil {
			return fmt.Errorf("invalid Vnode GUID: %v", err)
		}
	}
	return nil
}

func (s *SNMP) initSNMPClient() (gosnmp.Handler, error) {
	client := s.newSnmpClient()

	client.SetTarget(s.Hostname)
	client.SetPort(uint16(s.Options.Port))
	client.SetRetries(s.Options.Retries)
	client.SetTimeout(time.Duration(s.Options.Timeout) * time.Second)
	client.SetMaxOids(s.Options.MaxOIDs)
	client.SetMaxRepetitions(uint32(s.Options.MaxRepetitions))

	ver := parseSNMPVersion(s.Options.Version)
	comm := s.Community

	switch ver {
	case gosnmp.Version1:
		client.SetCommunity(comm)
		client.SetVersion(gosnmp.Version1)
	case gosnmp.Version2c:
		client.SetCommunity(comm)
		client.SetVersion(gosnmp.Version2c)
	case gosnmp.Version3:
		if s.User.Name == "" {
			return nil, errors.New("username is required for SNMPv3")
		}
		client.SetVersion(gosnmp.Version3)
		client.SetSecurityModel(gosnmp.UserSecurityModel)
		client.SetMsgFlags(parseSNMPv3SecurityLevel(s.User.SecurityLevel))
		client.SetSecurityParameters(&gosnmp.UsmSecurityParameters{
			UserName:                 s.User.Name,
			AuthenticationProtocol:   parseSNMPv3AuthProtocol(s.User.AuthProto),
			AuthenticationPassphrase: s.User.AuthKey,
			PrivacyProtocol:          parseSNMPv3PrivProtocol(s.User.PrivProto),
			PrivacyPassphrase:        s.User.PrivKey,
		})
	default:
		return nil, fmt.Errorf("invalid SNMP version: %s", s.Options.Version)
	}

	s.Info(snmpClientConnInfo(client))

	return client, nil
}

func (s *SNMP) initNetIfaceFilters() (matcher.Matcher, matcher.Matcher, error) {
	byName, byType := matcher.FALSE(), matcher.FALSE()

	if v := s.NetworkInterfaceFilter.ByName; v != "" {
		m, err := matcher.NewSimplePatternsMatcher(v)
		if err != nil {
			return nil, nil, err
		}
		byName = m
	}

	if v := s.NetworkInterfaceFilter.ByType; v != "" {
		m, err := matcher.NewSimplePatternsMatcher(v)
		if err != nil {
			return nil, nil, err
		}
		byType = m
	}

	return byName, byType, nil
}

func (s *SNMP) initOIDs() (oids []string) {
	for _, c := range *s.charts {
		for _, d := range c.Dims {
			oids = append(oids, d.ID)
		}
	}
	return oids
}

func parseSNMPVersion(version string) gosnmp.SnmpVersion {
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

func parseSNMPv3SecurityLevel(level string) gosnmp.SnmpV3MsgFlags {
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

func parseSNMPv3AuthProtocol(protocol string) gosnmp.SnmpV3AuthProtocol {
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

func parseSNMPv3PrivProtocol(protocol string) gosnmp.SnmpV3PrivProtocol {
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

func snmpClientConnInfo(c gosnmp.Handler) string {
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
