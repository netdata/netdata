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
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

func loadTopologyVLANContextProfiles(dev ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile {
	return resolveTopologyVLANProfileView(dev).Profiles()
}

func resolveTopologyVLANProfileView(dev ddsnmp.DeviceConnectionInfo) ddsnmp.ProjectedView {
	return ddsnmp.DefaultCatalog().Resolve(ddsnmp.ResolveRequest{
		SysObjectID:    dev.SysObjectID,
		SysDescr:       dev.SysDescr,
		ManualProfiles: dev.ManualProfiles,
		ManualPolicy:   ddsnmp.ManualProfileAugment,
	}).Project(ddsnmp.ConsumerTopology).FilterByKind(vlanScopableKinds)
}

type topologyVLANProgress struct {
	client       topologyAcquisitionPhaseEvidence
	connect      topologyAcquisitionPhaseEvidence
	collection   topologyAcquisitionPhaseEvidence
	interruption snmputils.Failure
	failures     ddsnmp.CollectionFailures
}

func newTopologyVLANProgress() topologyVLANProgress {
	return topologyVLANProgress{
		client:     notObservedAcquisitionPhase(),
		connect:    notObservedAcquisitionPhase(),
		collection: notObservedAcquisitionPhase(),
	}
}

func collectTopologyVLANContext(
	ctx context.Context,
	c *Collector,
	dev ddsnmp.DeviceConnectionInfo,
	vlanID string,
	profiles []*ddsnmp.Profile,
	observer ddsnmpcollector.AcquisitionObserver,
) ([]*ddsnmp.ProfileMetrics, topologyVLANProgress, error) {
	progress := newTopologyVLANProgress()
	if strings.TrimSpace(vlanID) == "" {
		err := fmt.Errorf("empty vlan id")
		progress.client = failedAcquisitionPhase(topologyAcquisitionFailureVLANIdentifier, err)
		return nil, progress, err
	}
	if _, err := strconv.Atoi(vlanID); err != nil {
		progress.client = failedAcquisitionPhase(topologyAcquisitionFailureVLANIdentifier, err)
		return nil, progress, fmt.Errorf("invalid vlan id '%s': %w", vlanID, err)
	}
	if err := ctx.Err(); err != nil {
		progress.interruption = snmputils.ClassifyFailure(err)
		return nil, progress, err
	}
	client, stopContextClose, progress, err := initTopologyVLANClient(ctx, c, dev, vlanID)
	if err != nil {
		return nil, progress, err
	}
	defer stopContextClose()
	defer func() { _ = client.Close() }()
	if err := ctx.Err(); err != nil {
		progress.interruption = snmputils.ClassifyFailure(err)
		return nil, progress, err
	}
	collector := c.newDdSnmpColl(ddsnmpcollector.Config{
		SnmpClient: client, Profiles: profiles, Log: c.Logger, SysObjectID: dev.SysObjectID, DisableBulkWalk: dev.DisableBulkWalk, InitialAcquisitionObserver: observer,
	})
	pms, err := collector.Collect()
	if source, ok := collector.(interface {
		CollectionFailures() ddsnmp.CollectionFailures
	}); ok {
		progress.failures = source.CollectionFailures()
	}
	if err != nil {
		progress.collection = failedAcquisitionPhase(topologyAcquisitionFailureCollection, err)
		progress.interruption = snmputils.ClassifyFailure(ctx.Err())
		return nil, progress, err
	}
	progress.collection = successfulAcquisitionPhase()
	if err := ctx.Err(); err != nil {
		progress.interruption = snmputils.ClassifyFailure(err)
		return nil, progress, err
	}
	return pms, progress, nil
}

func initTopologyVLANClient(
	ctx context.Context,
	c *Collector,
	dev ddsnmp.DeviceConnectionInfo,
	vlanID string,
) (gosnmp.Handler, func(), topologyVLANProgress, error) {
	progress := newTopologyVLANProgress()
	if err := ctx.Err(); err != nil {
		progress.interruption = snmputils.ClassifyFailure(err)
		return nil, func() {}, progress, err
	}
	client, err := newSNMPClientFromDeviceInfo(c.newSnmpClient, dev)
	if err != nil {
		progress.client = failedAcquisitionPhase(topologyAcquisitionFailureClientConfiguration, err)
		return nil, func() {}, progress, err
	}
	progress.client = successfulAcquisitionPhase()
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
		progress.connect = failedAcquisitionPhase(topologyAcquisitionFailureConnect, err)
		progress.interruption = snmputils.ClassifyFailure(ctx.Err())
		return nil, func() {}, progress, err
	}
	progress.connect = successfulAcquisitionPhase()
	return client, closeSNMPClientOnContextCancel(ctx, client), progress, nil
}
