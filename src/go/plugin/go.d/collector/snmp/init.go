// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/netdata/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/pinger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

func (c *Collector) validateConfig() error {
	if c.Hostname == "" {
		return snmputils.WithFailure(errors.New("SNMP hostname is required"), "configuration", "missing_hostname")
	}
	// RFC 3416 bounds max-repetitions at 2147483647; 0 and negative values
	// disable bulk walks.
	if c.Options.MaxRepetitions > math.MaxInt32 {
		return snmputils.WithFailure(
			fmt.Errorf("max_repetitions %d is more than %d", c.Options.MaxRepetitions, math.MaxInt32),
			"configuration",
			"invalid_configuration",
		)
	}
	if c.Vnode == "" && c.LocalVnode.GUID != "" {
		if err := uuid.Validate(c.LocalVnode.GUID); err != nil {
			return snmputils.WithFailure(fmt.Errorf("invalid Vnode GUID: %v", err), "configuration", "invalid_vnode_id")
		}
	}
	return nil
}

// configuredSNMPClient returns the SNMP client configuration the job's options
// describe; the log line shows its fields, never the configuration itself
// (it holds the community and the passphrases).
func (c *Collector) configuredSNMPClient() (gosnmp.Config, error) {
	cfg, err := c.snmpClientConfig()
	if err != nil {
		return gosnmp.Config{}, err
	}

	c.Infof("SNMP client target=%q port=%d version=%s context=%q", cfg.Target, cfg.Port, cfg.Version, cfg.ContextName)

	return cfg, nil
}

func (c *Collector) snmpClientConfig() (gosnmp.Config, error) {
	cfg := gosnmp.Config{
		Target:  c.Hostname,
		Port:    uint16(c.Options.Port),
		Retries: c.Options.Retries,
		Timeout: time.Duration(c.Options.Timeout) * time.Second,
		MaxOids: c.Options.MaxOIDs,
	}

	switch ver := snmputils.ParseSNMPVersion(c.Options.Version); ver {
	case gosnmp.Version1, gosnmp.Version2c:
		cfg.Version, cfg.Community = ver, c.Community
	case gosnmp.Version3:
		if c.User.Name == "" {
			return gosnmp.Config{}, snmputils.WithFailure(errors.New("username is required for SNMPv3"), "client", "missing_v3_username")
		}
		cfg.Version = gosnmp.Version3
		cfg.SecurityLevel = snmputils.ParseSNMPv3SecurityLevel(c.User.SecurityLevel)
		cfg.User = gosnmp.User{
			Name:           c.User.Name,
			AuthProtocol:   snmputils.ParseSNMPv3AuthProtocol(c.User.AuthProto),
			AuthPassphrase: c.User.AuthKey,
			PrivProtocol:   snmputils.ParseSNMPv3PrivProtocol(c.User.PrivProto),
			PrivPassphrase: c.User.PrivKey,
		}
		// Context name addresses a specific MIB view on multi-context
		// agents (virtual routers, logical partitions, snmpsim virtual
		// devices). Empty string means the default context.
		cfg.ContextName = c.User.ContextName
	default:
		return gosnmp.Config{}, snmputils.WithFailure(fmt.Errorf("invalid SNMP version: %s", c.Options.Version), "client", "invalid_snmp_version")
	}

	return cfg, nil
}

func (c *Collector) initPinger() (pinger.Client, error) {
	// base timeout = update_every seconds
	timeout := time.Duration(c.UpdateEvery) * time.Second

	// clamp between 1s and 3s
	const minTimeout = time.Second
	const maxTimeout = 3 * time.Second
	timeout = max(min(timeout, maxTimeout), minTimeout)

	return c.newPinger(pinger.Config{
		Probe: pinger.ProbeConfig{
			Network:    c.Ping.Network,
			Interface:  c.Ping.Interface,
			Privileged: c.Ping.Privileged,
			Packets:    c.Ping.Packets,
			Interval:   c.Ping.Interval,
			Timeout:    timeout,
		},
	}, c.Logger)
}
