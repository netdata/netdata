// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"errors"
	"fmt"
	"time"

	"github.com/gosnmp/gosnmp"
)

var newSNMPClient = gosnmp.NewHandler

func (s *SNMP) validateConfig() error {
	if len(s.ChartsInput) == 0 {
		return errors.New("'charts' are required but not set")
	}

	if s.Options.Version == gosnmp.Version3.String() {
		if s.User.Name == "" {
			return errors.New("'user.name' is required when using SNMPv3 but not set")
		}
		if _, err := parseSNMPv3SecurityLevel(s.User.SecurityLevel); err != nil {
			return err
		}
		if _, err := parseSNMPv3AuthProtocol(s.User.AuthProto); err != nil {
			return err
		}
		if _, err := parseSNMPv3PrivProtocol(s.User.PrivProto); err != nil {
			return err
		}
	}

	return nil
}

func (s *SNMP) initSNMPClient() (gosnmp.Handler, error) {
	client := newSNMPClient()

	if client.SetTarget(s.Hostname); client.Target() == "" {
		s.Warningf("'hostname' not set, using the default value: '%s'", defaultHostname)
		client.SetTarget(defaultHostname)
	}
	if client.SetPort(uint16(s.Options.Port)); client.Port() <= 0 || client.Port() > 65535 {
		s.Warningf("'options.port' is invalid, changing to the default value: '%d' => '%d'", s.Options.Port, defaultPort)
		client.SetPort(defaultPort)
	}
	if client.SetRetries(s.Options.Retries); client.Retries() < 1 || client.Retries() > 10 {
		s.Warningf("'options.retries' is invalid, changing to the default value: '%d' => '%d'", s.Options.Retries, defaultRetries)
		client.SetRetries(defaultRetries)
	}
	if client.SetTimeout(time.Duration(s.Options.Timeout) * time.Second); client.Timeout().Seconds() < 1 {
		s.Warningf("'options.timeout' is invalid, changing to the default value: '%d' => '%d'", s.Options.Timeout, defaultTimeout)
		client.SetTimeout(defaultTimeout * time.Second)
	}
	if client.SetMaxOids(s.Options.MaxOIDs); client.MaxOids() < 1 {
		s.Warningf("'options.max_request_size' is invalid, changing to the default value: '%d' => '%d'", s.Options.MaxOIDs, defaultMaxOIDs)
		client.SetMaxOids(defaultMaxOIDs)
	}

	ver, err := parseSNMPVersion(s.Options.Version)
	if err != nil {
		s.Warningf("'options.version' is invalid, changing to the default value: '%s' => '%s'",
			s.Options.Version, defaultVersion)
		ver = defaultVersion
	}
	comm := s.Community
	if comm == "" && (ver <= gosnmp.Version2c) {
		s.Warningf("'community' not set, using the default value: '%s'", defaultCommunity)
		comm = defaultCommunity
	}

	switch ver {
	case gosnmp.Version1:
		client.SetCommunity(comm)
		client.SetVersion(gosnmp.Version1)
	case gosnmp.Version2c:
		client.SetCommunity(comm)
		client.SetVersion(gosnmp.Version2c)
	case gosnmp.Version3:
		client.SetVersion(gosnmp.Version3)
		client.SetSecurityModel(gosnmp.UserSecurityModel)
		client.SetMsgFlags(safeParseSNMPv3SecurityLevel(s.User.SecurityLevel))
		client.SetSecurityParameters(&gosnmp.UsmSecurityParameters{
			UserName:                 s.User.Name,
			AuthenticationProtocol:   safeParseSNMPv3AuthProtocol(s.User.AuthProto),
			AuthenticationPassphrase: s.User.AuthKey,
			PrivacyProtocol:          safeParseSNMPv3PrivProtocol(s.User.PrivProto),
			PrivacyPassphrase:        s.User.PrivKey,
		})
	default:
		return nil, fmt.Errorf("invalid SNMP version: %s", s.Options.Version)
	}

	return client, nil
}

func (s *SNMP) initOIDs() (oids []string) {
	for _, c := range *s.charts {
		for _, d := range c.Dims {
			oids = append(oids, d.ID)
		}
	}
	return oids
}

func parseSNMPVersion(version string) (gosnmp.SnmpVersion, error) {
	switch version {
	case "0", "1":
		return gosnmp.Version1, nil
	case "2", "2c", "":
		return gosnmp.Version2c, nil
	case "3":
		return gosnmp.Version3, nil
	default:
		return gosnmp.Version2c, fmt.Errorf("invalid snmp version value (%s)", version)
	}
}

func safeParseSNMPv3SecurityLevel(level string) gosnmp.SnmpV3MsgFlags {
	v, _ := parseSNMPv3SecurityLevel(level)
	return v
}

func parseSNMPv3SecurityLevel(level string) (gosnmp.SnmpV3MsgFlags, error) {
	switch level {
	case "1", "none", "noAuthNoPriv", "":
		return gosnmp.NoAuthNoPriv, nil
	case "2", "authNoPriv":
		return gosnmp.AuthNoPriv, nil
	case "3", "authPriv":
		return gosnmp.AuthPriv, nil
	default:
		return gosnmp.NoAuthNoPriv, fmt.Errorf("invalid snmpv3 user security level value (%s)", level)
	}
}

func safeParseSNMPv3AuthProtocol(protocol string) gosnmp.SnmpV3AuthProtocol {
	v, _ := parseSNMPv3AuthProtocol(protocol)
	return v
}

func parseSNMPv3AuthProtocol(protocol string) (gosnmp.SnmpV3AuthProtocol, error) {
	switch protocol {
	case "1", "none", "noAuth", "":
		return gosnmp.NoAuth, nil
	case "2", "md5":
		return gosnmp.MD5, nil
	case "3", "sha":
		return gosnmp.SHA, nil
	case "4", "sha224":
		return gosnmp.SHA224, nil
	case "5", "sha256":
		return gosnmp.SHA256, nil
	case "6", "sha384":
		return gosnmp.SHA384, nil
	case "7", "sha512":
		return gosnmp.SHA512, nil
	default:
		return gosnmp.NoAuth, fmt.Errorf("invalid snmpv3 user auth protocol value (%s)", protocol)
	}
}

func safeParseSNMPv3PrivProtocol(protocol string) gosnmp.SnmpV3PrivProtocol {
	v, _ := parseSNMPv3PrivProtocol(protocol)
	return v
}

func parseSNMPv3PrivProtocol(protocol string) (gosnmp.SnmpV3PrivProtocol, error) {
	switch protocol {
	case "1", "none", "noPriv", "":
		return gosnmp.NoPriv, nil
	case "2", "des":
		return gosnmp.DES, nil
	case "3", "aes":
		return gosnmp.AES, nil
	case "4", "aes192":
		return gosnmp.AES192, nil
	case "5", "aes256":
		return gosnmp.AES256, nil
	case "6", "aes192c":
		return gosnmp.AES192C, nil
	case "7", "aes256c":
		return gosnmp.AES256C, nil
	default:
		return gosnmp.NoPriv, fmt.Errorf("invalid snmpv3 user priv protocol value (%s)", protocol)
	}
}
