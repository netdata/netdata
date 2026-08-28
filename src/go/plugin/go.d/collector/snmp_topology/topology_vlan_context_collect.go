// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
)

func loadTopologyVLANContextProfiles(dev ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile {
	return ddsnmp.DefaultCatalog().Resolve(ddsnmp.ResolveRequest{
		SysObjectID:    dev.SysObjectID,
		SysDescr:       dev.SysDescr,
		ManualProfiles: dev.ManualProfiles,
		ManualPolicy:   ddsnmp.ManualProfileAugment,
	}).Project(ddsnmp.ConsumerTopology).FilterByKind(vlanScopableKinds).Profiles()
}

func collectTopologyVLANContext(
	ctx context.Context,
	c *Collector,
	dev ddsnmp.DeviceConnectionInfo,
	vlanID string,
	profiles []*ddsnmp.Profile,
	observer ddsnmpcollector.AcquisitionObserver,
) ([]*ddsnmp.ProfileMetrics, topologyAcquisitionFailureClass, error) {
	if strings.TrimSpace(vlanID) == "" {
		return nil, topologyAcquisitionFailureVLANIdentifier, fmt.Errorf("empty vlan id")
	}
	if _, err := strconv.Atoi(vlanID); err != nil {
		return nil, topologyAcquisitionFailureVLANIdentifier, fmt.Errorf("invalid vlan id '%s': %w", vlanID, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, topologyAcquisitionFailureCollection, err
	}

	snmpClient, stopContextClose, failure, err := initTopologyVLANClient(ctx, c, dev, vlanID)
	if err != nil {
		return nil, failure, err
	}
	defer stopContextClose()
	defer func() {
		_ = snmpClient.Close()
	}()

	if err := ctx.Err(); err != nil {
		return nil, topologyAcquisitionFailureCollection, err
	}

	vlanCollector := c.newDdSnmpColl(ddsnmpcollector.Config{
		SnmpClient:                 snmpClient,
		Profiles:                   profiles,
		Log:                        c.Logger,
		SysObjectID:                dev.SysObjectID,
		DisableBulkWalk:            dev.DisableBulkWalk,
		InitialAcquisitionObserver: observer,
	})

	pms, err := vlanCollector.Collect()
	if err != nil {
		return nil, topologyAcquisitionFailureCollection, err
	}
	if err := ctx.Err(); err != nil {
		return nil, topologyAcquisitionFailureCollection, err
	}
	return pms, topologyAcquisitionFailureNone, nil
}

func initTopologyVLANClient(
	ctx context.Context,
	c *Collector,
	dev ddsnmp.DeviceConnectionInfo,
	vlanID string,
) (gosnmp.Handler, func(), topologyAcquisitionFailureClass, error) {
	if err := ctx.Err(); err != nil {
		return nil, func() {}, topologyAcquisitionFailureCollection, err
	}

	client, err := newSNMPClientFromDeviceInfo(c.newSnmpClient, dev)
	if err != nil {
		return nil, func() {}, topologyAcquisitionFailureClientConfiguration, err
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
		return nil, func() {}, topologyAcquisitionFailureConnect, err
	}

	stopContextClose := closeSNMPClientOnContextCancel(ctx, client)
	return client, stopContextClose, topologyAcquisitionFailureNone, nil
}
