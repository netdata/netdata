// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestCollector_Collect_RegularScalarFallbackKeepsFirstSuccessfulMetric(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const (
		primaryOID  = "1.3.6.1.4.1.99999.1.0"
		fallbackOID = "1.3.6.1.4.1.99999.2.0"
	)

	expectSNMPGet(mockHandler, []string{primaryOID, fallbackOID}, []gosnmp.SnmpPDU{
		createGauge32PDU(primaryOID, 10),
		createGauge32PDU(fallbackOID, 20),
	})

	profile := createTestProfile("scalar-fallback.yaml", []ddprofiledefinition.MetricsConfig{
		createScalarMetric(primaryOID, "systemUptime"),
		createScalarMetric(fallbackOID, "systemUptime"),
	})
	profile.Definition.VirtualMetrics = []ddprofiledefinition.VirtualMetricConfig{
		{
			Name: "selectedUptime",
			Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
				{Metric: "systemUptime"},
			},
		},
	}

	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
	})
	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)

	pm := results[0]
	require.Len(t, pm.Metrics, 2)
	assert.Equal(t, ddsnmp.Metric{Name: "systemUptime", Value: 10, MetricType: "gauge", Profile: pm}, pm.Metrics[0])
	assert.Equal(t, ddsnmp.Metric{Name: "selectedUptime", Value: 10, MetricType: "gauge", Profile: pm}, pm.Metrics[1])
	assert.Equal(t, int64(1), pm.Stats.Metrics.Scalar)
	assert.Equal(t, int64(1), pm.Stats.Metrics.Virtual)
	assert.Equal(t, int64(2), pm.Stats.SNMP.GetOIDs)
}

func TestSystemBaseProfile_ScalarFallbackOrder(t *testing.T) {
	profile, err := ddsnmp.LoadProfileByName("_system-base")
	require.NoError(t, err)

	var uptimeOIDs []string
	for _, metric := range profile.Definition.Metrics {
		if metric.IsScalar() && metric.Symbol.Name == "systemUptime" {
			uptimeOIDs = append(uptimeOIDs, metric.Symbol.OID)
		}
	}
	assert.Equal(t, []string{
		"1.3.6.1.6.3.10.2.1.3.0", // snmpEngineTime
		"1.3.6.1.2.1.25.1.1.0",   // hrSystemUptime
		"1.3.6.1.2.1.1.3.0",      // sysUpTime
	}, uptimeOIDs)
}

func TestCollector_Collect_RegularScalarFallbackFallsThrough(t *testing.T) {
	const (
		primaryOID  = "1.3.6.1.4.1.99999.3.0"
		fallbackOID = "1.3.6.1.4.1.99999.4.0"
	)

	tests := map[string]struct {
		primaryConfig   ddprofiledefinition.MetricsConfig
		primaryPDU      gosnmp.SnmpPDU
		missingErrors   int64
		processingError int64
	}{
		"missing primary": {
			primaryConfig: createScalarMetric(primaryOID, "systemUptime"),
			primaryPDU:    createNoSuchObjectPDU(primaryOID),
			missingErrors: 1,
		},
		"unusable primary": {
			primaryConfig:   createScalarMetric(primaryOID, "systemUptime"),
			primaryPDU:      createStringPDU(primaryOID, "not-a-number"),
			processingError: 1,
		},
		"soft-skipped primary": {
			primaryConfig: ddprofiledefinition.MetricsConfig{
				Symbol: ddprofiledefinition.SymbolConfig{
					OID:    primaryOID,
					Name:   "systemUptime",
					Format: "text_date",
				},
			},
			primaryPDU: createStringPDU(primaryOID, "never"),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl, mockHandler := setupMockHandler(t)
			defer ctrl.Finish()

			expectSNMPGet(mockHandler, []string{primaryOID, fallbackOID}, []gosnmp.SnmpPDU{
				test.primaryPDU,
				createGauge32PDU(fallbackOID, 20),
			})

			profile := createTestProfile("scalar-fallback.yaml", []ddprofiledefinition.MetricsConfig{
				test.primaryConfig,
				createScalarMetric(fallbackOID, "systemUptime"),
			})
			collector := New(Config{
				SnmpClient: mockHandler,
				Profiles:   []*ddsnmp.Profile{profile},
				Log:        logger.New(),
			})
			results, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, results, 1)

			pm := results[0]
			require.Len(t, pm.Metrics, 1)
			assert.Equal(t, "systemUptime", pm.Metrics[0].Name)
			assert.EqualValues(t, 20, pm.Metrics[0].Value)
			assert.Equal(t, int64(1), pm.Stats.Metrics.Scalar)
			assert.Equal(t, test.missingErrors, pm.Stats.Errors.MissingOIDs)
			assert.Equal(t, test.processingError, pm.Stats.Errors.Processing.Scalar)
			assert.Equal(t, int64(2), pm.Stats.SNMP.GetOIDs)
		})
	}
}

func TestCollector_KeepFirstRegularScalarMetricByName(t *testing.T) {
	collector := &Collector{}

	first := []ddsnmp.Metric{
		{Name: "fallback", Value: 1},
		{Name: "distinct", Value: 2},
		{Name: "fallback", Value: 3},
	}
	assert.Equal(t, []ddsnmp.Metric{
		{Name: "fallback", Value: 1},
		{Name: "distinct", Value: 2},
	}, collector.keepFirstRegularScalarMetricByName(first))

	second := []ddsnmp.Metric{
		{Name: "fallback", Value: 4},
		{Name: "fallback", Value: 5},
	}
	assert.Equal(t, []ddsnmp.Metric{
		{Name: "fallback", Value: 4},
	}, collector.keepFirstRegularScalarMetricByName(second), "winner state must reset for each profile")
}

func TestCollector_Collect_RegularScalarFallbackDoesNotAffectTopologyScalars(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const (
		firstOID  = "1.3.6.1.4.1.99999.10.0"
		secondOID = "1.3.6.1.4.1.99999.11.0"
	)

	expectSNMPGet(mockHandler, []string{firstOID, secondOID}, []gosnmp.SnmpPDU{
		createGauge32PDU(firstOID, 1),
		createGauge32PDU(secondOID, 2),
	})

	profile := &ddsnmp.Profile{
		SourceFile: "topology-scalars.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{
			Topology: []ddprofiledefinition.TopologyConfig{
				{
					Kind: ddprofiledefinition.KindIfStatus,
					MetricsConfig: ddprofiledefinition.MetricsConfig{
						Symbol: ddprofiledefinition.SymbolConfig{OID: firstOID, Name: "if_status"},
					},
				},
				{
					Kind: ddprofiledefinition.KindIfStatus,
					MetricsConfig: ddprofiledefinition.MetricsConfig{
						Symbol: ddprofiledefinition.SymbolConfig{OID: secondOID, Name: "if_status"},
					},
				},
			},
		},
	}

	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
	})
	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Empty(t, results[0].Metrics)
	require.Len(t, results[0].TopologyMetrics, 2)
	assert.EqualValues(t, 1, results[0].TopologyMetrics[0].Value)
	assert.EqualValues(t, 2, results[0].TopologyMetrics[1].Value)
}

func BenchmarkCollector_CollectScalarFallbacks(b *testing.B) {
	for _, names := range []int{16, 64, 256} {
		b.Run(fmt.Sprintf("names=%d/alternatives=3", names), func(b *testing.B) {
			ctrl := gomock.NewController(b)
			defer ctrl.Finish()

			mockHandler := snmpmock.NewMockHandler(ctrl)
			mockHandler.EXPECT().MaxOids().Return(4096).AnyTimes()
			device := newStatefulSNMPDevice()
			device.install(mockHandler)

			metrics := make([]ddprofiledefinition.MetricsConfig, 0, names*3)
			for i := range names {
				name := fmt.Sprintf("metric_%d", i)
				for alt := range 3 {
					oid := fmt.Sprintf("1.3.6.1.4.1.99999.%d.%d.0", i+1, alt+1)
					metrics = append(metrics, createScalarMetric(oid, name))
					device.set(createGauge32PDU(oid, uint(i+alt+1)))
				}
			}

			collector := New(Config{
				SnmpClient: mockHandler,
				Profiles:   []*ddsnmp.Profile{createTestProfile("benchmark-fallbacks.yaml", metrics)},
				Log:        logger.New(),
			})

			_, err := collector.Collect()
			require.NoError(b, err)

			b.ReportAllocs()
			b.ReportMetric(float64(names), "names/op")
			b.ResetTimer()
			for range b.N {
				results, err := collector.Collect()
				if err != nil || len(results) != 1 {
					b.Fatalf("collect: results=%d err=%v", len(results), err)
				}
				runtime.KeepAlive(results)
			}
		})
	}
}
