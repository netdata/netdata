// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologydiag"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
	"github.com/stretchr/testify/require"
)

func TestVLANPreClientCancellationDoesNotReportCollectionFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, progress, err := collectTopologyVLANContext(ctx, newTestSNMPTopologyCollector(), ddsnmp.DeviceConnectionInfo{}, "100", nil, nil)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, topologydiag.AcquisitionPhaseNotObserved, progress.client.Outcome)
	require.Equal(t, topologydiag.AcquisitionPhaseNotObserved, progress.connect.Outcome)
	require.Equal(t, topologydiag.AcquisitionPhaseNotObserved, progress.collection.Outcome)
	require.Equal(t, "cancelled", progress.interruption.Reason)
}

func TestNonstandardPacketStatusSurvivesArchive(t *testing.T) {
	handler := snmpmock.NewMockHandler(gomock.NewController(t))
	handler.EXPECT().MaxOids().Return(20)
	handler.EXPECT().Version().Return(gosnmp.Version2c)
	handler.EXPECT().Get(gomock.Any()).Return(&gosnmp.SnmpPacket{Error: gosnmp.SNMPError(255), ErrorIndex: 2}, nil)
	_, err := snmputils.GetSysInfo(handler)
	require.Error(t, err)
	failure := snmputils.ClassifyFailure(err)
	require.EqualValues(t, 255, failure.PacketStatus)
	store := ddsnmp.NewDeviceStore()
	writer := store.ReplaceJob("", "device", ddsnmp.DeviceLifecycleInfo{Hostname: "192.0.2.1"}, ddsnmp.DeviceLifecycleStatus{}, nil)
	writer.RecordLifecycle(ddsnmp.DeviceLifecycleStatus{Phase: ddsnmp.DeviceLifecyclePhaseCheck, Outcome: ddsnmp.DeviceLifecycleOutcomeFailed, Failure: failure})
	lifecycle := snmpdiag.CaptureLifecycle(store)
	archive, err := InspectDiagnosticDocument(snmpdiag.Document{Format: snmpdiag.Format, Version: snmpdiag.Version, Snapshot: snmpdiag.Snapshot{Lifecycle: lifecycle}})
	require.NoError(t, err)
	require.NotNil(t, archive)
}
