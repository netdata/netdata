// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/netdata/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologydiag"
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
	sources      []*ddsnmp.SourceOperation
	client       topologydiag.AcquisitionPhaseEvidence
	connect      topologydiag.AcquisitionPhaseEvidence
	collection   topologydiag.AcquisitionPhaseEvidence
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
	registrationID ddsnmp.DeviceRegistrationID,
	dev ddsnmp.DeviceConnectionInfo,
	vlanID string,
	profiles []*ddsnmp.Profile,
	observer ddsnmpcollector.AcquisitionObserver,
) ([]*ddsnmp.ProfileMetrics, topologyVLANProgress, error) {
	progress := newTopologyVLANProgress()
	if strings.TrimSpace(vlanID) == "" {
		err := fmt.Errorf("empty vlan id")
		progress.client = failedAcquisitionPhase(topologydiag.AcquisitionFailureVLANIdentifier, err)
		return nil, progress, err
	}
	if _, err := strconv.Atoi(vlanID); err != nil {
		progress.client = failedAcquisitionPhase(topologydiag.AcquisitionFailureVLANIdentifier, err)
		return nil, progress, fmt.Errorf("invalid vlan id '%s': %w", vlanID, err)
	}
	if err := ctx.Err(); err != nil {
		progress.interruption = snmputils.ClassifyFailure(err)
		return nil, progress, err
	}
	client, progress, err := initTopologyVLANClient(ctx, c, dev, vlanID)
	if err != nil {
		return nil, progress, err
	}
	defer func() { _ = client.Close() }()
	if err := ctx.Err(); err != nil {
		progress.interruption = snmputils.ClassifyFailure(err)
		return nil, progress, err
	}
	sourceRecorder := &ddsnmpcollector.SourceRecorder{}
	collector := c.newDdSnmpColl(ddsnmpcollector.Config{
		SnmpClient:          sourceRecorder.Wrap(client),
		Version:             snmputils.ParseSNMPVersion(dev.SNMPVersion),
		MaxOIDs:             dev.MaxOIDs,
		MaxRepetitions:      dev.MaxRepetitions,
		Profiles:            profiles,
		SysObjectID:         dev.SysObjectID,
		DisableBulkWalk:     dev.DisableBulkWalk,
		AcquisitionObserver: observer,
		Log: c.Logger.With(
			slog.String("device", dev.Hostname),
			slog.String("registration", registrationID.String()),
			slog.String("context", "vlan"),
			slog.String("vlan", vlanID),
		),
		LogKeyPrefix: "snmp_topology:registration:" + registrationID.String() + ":vlan:" + vlanID + ":",
	})
	pms, err := collector.Collect(ctx)
	progress.sources = sourceRecorder.Finish()
	if source, ok := collector.(interface {
		CollectionFailures() ddsnmp.CollectionFailures
	}); ok {
		progress.failures = source.CollectionFailures()
	}
	if err != nil {
		progress.collection = failedAcquisitionPhase(topologydiag.AcquisitionFailureCollection, err)
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
) (snmputils.Session, topologyVLANProgress, error) {
	progress := newTopologyVLANProgress()
	if err := ctx.Err(); err != nil {
		progress.interruption = snmputils.ClassifyFailure(err)
		return nil, progress, err
	}
	client, err := newVLANContextSession(c.newSnmpClient, dev, vlanID)
	if err != nil {
		progress.client = failedAcquisitionPhase(topologydiag.AcquisitionFailureClientConfiguration, err)
		return nil, progress, err
	}
	progress.client = successfulAcquisitionPhase()
	if err := client.Connect(ctx); err != nil {
		progress.connect = failedAcquisitionPhase(topologydiag.AcquisitionFailureConnect, err)
		progress.interruption = snmputils.ClassifyFailure(ctx.Err())
		return nil, progress, err
	}
	progress.connect = successfulAcquisitionPhase()
	return client, progress, nil
}

// newVLANContextSession makes the SNMP client of one VLAN context: the
// device's settings with the VLAN's context name (SNMPv3) or the VLAN-indexed
// community "<community>@<vlan>" (SNMPv1 and SNMPv2c).
func newVLANContextSession(
	newClient func(gosnmp.Config) (snmputils.Session, error),
	dev ddsnmp.DeviceConnectionInfo,
	vlanID string,
) (snmputils.Session, error) {
	cfg, err := snmpClientConfigFromDeviceInfo(dev)
	if err != nil {
		return nil, err
	}
	if cfg.Version == gosnmp.Version3 {
		cfg.ContextName = "vlan-" + vlanID
	} else {
		cfg.Community = dev.Community + "@" + vlanID
	}
	return newClient(cfg)
}
