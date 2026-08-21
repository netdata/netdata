// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"slices"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestCollector_CollectDeviceMetadata_SynologyProfile(t *testing.T) {
	const sysObjectID = "1.3.6.1.4.1.8072.3.2.10"

	profile := mustLoadSynologyProfile(t)
	device := profile.Definition.Metadata[ddprofiledefinition.MetadataDeviceResource]
	for name := range device.Fields {
		if !slices.Contains([]string{"vendor", "type", "model", "serial_number", "version"}, name) {
			delete(device.Fields, name)
		}
	}
	profile.Definition.Metadata = ddprofiledefinition.MetadataConfig{
		ddprofiledefinition.MetadataDeviceResource: device,
	}

	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPGet(mockHandler, []string{
		"1.3.6.1.4.1.6574.1.5.1.0",
		"1.3.6.1.4.1.6574.1.5.2.0",
		"1.3.6.1.4.1.6574.1.5.3.0",
	}, []gosnmp.SnmpPDU{
		createStringPDU("1.3.6.1.4.1.6574.1.5.1.0", "Test NAS"),
		createStringPDU("1.3.6.1.4.1.6574.1.5.2.0", "TEST-SERIAL"),
		createStringPDU("1.3.6.1.4.1.6574.1.5.3.0", "DSM test"),
	})

	collector := New(Config{
		SnmpClient:  mockHandler,
		Profiles:    []*ddsnmp.Profile{profile},
		Log:         logger.New(),
		SysObjectID: sysObjectID,
	})

	got, err := collector.CollectDeviceMetadata()
	require.NoError(t, err)
	assert.Equal(t, map[string]ddsnmp.MetaTag{
		"vendor":        {Value: "Synology", IsExactMatch: true},
		"type":          {Value: "Storage", IsExactMatch: true},
		"model":         {Value: "Test NAS", IsExactMatch: true},
		"serial_number": {Value: "TEST-SERIAL", IsExactMatch: true},
		"version":       {Value: "DSM test", IsExactMatch: true},
	}, got)
}

func TestScalarCollector_Collect_SynologyUtilizationProfile(t *testing.T) {
	const (
		cpuOID    = "1.3.6.1.4.1.6574.1.7.1.0"
		memoryOID = "1.3.6.1.4.1.6574.1.7.2.0"
	)

	profile := mustLoadSynologyProfile(t)
	profile.Definition.Metrics = slices.DeleteFunc(profile.Definition.Metrics, func(metric ddprofiledefinition.MetricsConfig) bool {
		return !slices.Contains([]string{"cpu.usage", "memory.usage"}, metric.Symbol.Name)
	})
	require.Len(t, profile.Definition.Metrics, 2)

	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPGet(mockHandler, []string{cpuOID, memoryOID}, []gosnmp.SnmpPDU{
		createIntegerPDU(cpuOID, 6),
		createIntegerPDU(memoryOID, 4),
	})

	collector := newScalarCollector(mockHandler, make(map[string]bool), logger.New())
	got, err := collector.collect(profile, &ddsnmp.CollectionStats{})
	require.NoError(t, err)
	assertMetricsEqual(t, []ddsnmp.Metric{
		{
			Name:        "cpu.usage",
			Description: "The current CPU utilization",
			Family:      "System/CPU/Usage",
			Unit:        "%",
			MetricType:  ddprofiledefinition.ProfileMetricTypeGauge,
			Value:       6,
		},
		{
			Name:        "memory.usage",
			Description: "Memory utilization",
			Family:      "System/Memory/Usage",
			Unit:        "%",
			MetricType:  ddprofiledefinition.ProfileMetricTypeGauge,
			Value:       4,
		},
	}, got)
}

func mustLoadSynologyProfile(t *testing.T) *ddsnmp.Profile {
	t.Helper()

	profile, err := ddsnmp.LoadProfileByName("synology-disk-station")
	require.NoError(t, err)
	return profile
}
