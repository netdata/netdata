// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
)

func loadTopologyVLANContextProfiles() ([]*ddsnmp.Profile, error) {
	names := []string{fdbArpProfileName, stpProfileName}
	profiles := make([]*ddsnmp.Profile, 0, len(names))
	for _, name := range names {
		profile, err := ddsnmp.LoadProfileByName(name)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}

	return ddsnmp.FinalizeProfiles(profiles), nil
}

func collectTopologyVLANContext(c *Collector, dev ddsnmp.DeviceConnectionInfo, vlanID string, profiles []*ddsnmp.Profile) ([]*ddsnmp.ProfileMetrics, error) {
	if strings.TrimSpace(vlanID) == "" {
		return nil, fmt.Errorf("empty vlan id")
	}
	if _, err := strconv.Atoi(vlanID); err != nil {
		return nil, fmt.Errorf("invalid vlan id '%s': %w", vlanID, err)
	}

	snmpClient, err := initTopologyVLANClient(c, dev, vlanID)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = snmpClient.Close()
	}()

	vlanCollector := c.newDdSnmpColl(ddsnmpcollector.Config{
		SnmpClient:      snmpClient,
		Profiles:        profiles,
		Log:             c.Logger,
		SysObjectID:     dev.SysObjectID,
		DisableBulkWalk: dev.DisableBulkWalk,
	})

	return vlanCollector.Collect()
}

func initTopologyVLANClient(c *Collector, dev ddsnmp.DeviceConnectionInfo, vlanID string) (gosnmp.Handler, error) {
	client, err := newSNMPClientFromDeviceInfo(c.newSnmpClient, dev)
	if err != nil {
		return nil, err
	}

	switch client.Version() {
	case gosnmp.Version3:
		client.SetContextName("vlan-" + vlanID)
	default:
		baseCommunity := client.Community()
		if baseCommunity == "" {
			baseCommunity = dev.Community
		}
		client.SetCommunity(baseCommunity + "@" + vlanID)
	}

	if dev.MaxRepetitions != 0 {
		client.SetMaxRepetitions(dev.MaxRepetitions)
	}

	if err := client.Connect(); err != nil {
		return nil, err
	}

	return client, nil
}
