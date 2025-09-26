// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/ping"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

func (c *Collector) validateConfig() error {
	if c.Hostname == "" {
		return errors.New("SNMP hostname is required")
	}
	if c.Vnode.GUID != "" {
		if err := uuid.Validate(c.Vnode.GUID); err != nil {
			return fmt.Errorf("invalid Vnode GUID: %v", err)
		}
	}
	return nil
}

func (c *Collector) initSNMPClient() (gosnmp.Handler, error) {
	client := c.newSnmpClient()

	client.SetTarget(c.Hostname)
	client.SetPort(uint16(c.Options.Port))
	client.SetRetries(c.Options.Retries)
	client.SetTimeout(time.Duration(c.Options.Timeout) * time.Second)
	client.SetMaxOids(c.Options.MaxOIDs)
	client.SetMaxRepetitions(uint32(c.Options.MaxRepetitions))

	ver := snmputils.ParseSNMPVersion(c.Options.Version)
	comm := c.Community

	switch ver {
	case gosnmp.Version1:
		client.SetCommunity(comm)
		client.SetVersion(gosnmp.Version1)
	case gosnmp.Version2c:
		client.SetCommunity(comm)
		client.SetVersion(gosnmp.Version2c)
	case gosnmp.Version3:
		if c.User.Name == "" {
			return nil, errors.New("username is required for SNMPv3")
		}
		client.SetVersion(gosnmp.Version3)
		client.SetSecurityModel(gosnmp.UserSecurityModel)
		client.SetMsgFlags(snmputils.ParseSNMPv3SecurityLevel(c.User.SecurityLevel))
		client.SetSecurityParameters(&gosnmp.UsmSecurityParameters{
			UserName:                 c.User.Name,
			AuthenticationProtocol:   snmputils.ParseSNMPv3AuthProtocol(c.User.AuthProto),
			AuthenticationPassphrase: c.User.AuthKey,
			PrivacyProtocol:          snmputils.ParseSNMPv3PrivProtocol(c.User.PrivProto),
			PrivacyPassphrase:        c.User.PrivKey,
		})
	default:
		return nil, fmt.Errorf("invalid SNMP version: %s", c.Options.Version)
	}

	c.Info(snmputils.SnmpClientConnInfo(client))

	return client, nil
}

func (c *Collector) initProber() (ping.Prober, error) {
	// base timeout = update_every seconds
	timeout := time.Duration(c.UpdateEvery) * time.Second

	// clamp between 1s and 3s
	const minTimeout = time.Second
	const maxTimeout = 3 * time.Second
	timeout = max(min(timeout, maxTimeout), minTimeout)

	conf := c.Ping.ProberConfig
	conf.Timeout = timeout

	return c.newProber(conf, c.Logger), nil
}
