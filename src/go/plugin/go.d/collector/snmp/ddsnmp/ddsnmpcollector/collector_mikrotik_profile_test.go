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
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/oldmetrix"
)

func TestCollector_Collect_MikroTikGaugeProfile(t *testing.T) {
	const (
		tableOID = "1.3.6.1.4.1.14988.1.1.3.100"
		nameOID  = tableOID + ".1.2"
		valueOID = tableOID + ".1.3"
		unitOID  = tableOID + ".1.4"
	)

	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPWalk(mockHandler, gosnmp.Version2c, tableOID, []gosnmp.SnmpPDU{
		createStringPDU(nameOID+".17", "cpu-temperature"),
		createIntegerPDU(valueOID+".17", 51),
		createIntegerPDU(unitOID+".17", 1),
		createStringPDU(nameOID+".18", "psu-state"),
		createIntegerPDU(valueOID+".18", 1),
		createIntegerPDU(unitOID+".18", 6),
		createStringPDU(nameOID+".7201", "jack-voltage"),
		createIntegerPDU(valueOID+".7201", 492),
		createIntegerPDU(unitOID+".7201", 3),
		createStringPDU(nameOID+".7202", "input-current"),
		createIntegerPDU(valueOID+".7202", 15),
		createIntegerPDU(unitOID+".7202", 4),
		createStringPDU(nameOID+".7203", "input-power"),
		createIntegerPDU(valueOID+".7203", 123),
		createIntegerPDU(unitOID+".7203", 5),
	})
	expectSNMPGet(mockHandler, []string{
		valueOID + ".17",
		valueOID + ".18",
		valueOID + ".7201",
		valueOID + ".7202",
		valueOID + ".7203",
	}, []gosnmp.SnmpPDU{
		createIntegerPDU(valueOID+".17", 52),
		createIntegerPDU(valueOID+".18", 0),
		createIntegerPDU(valueOID+".7201", 501),
		createIntegerPDU(valueOID+".7202", 21),
		createIntegerPDU(valueOID+".7203", 131),
	})

	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{mustLoadMikroTikGaugeProfile(t)},
		Log:        logger.New(),
	})

	tests := []struct {
		name string
		want []ddsnmp.Metric
	}{
		{
			name: "initial walk",
			want: mikrotikGaugeMetrics(51, 1, 49, 1, 12),
		},
		{
			name: "cached get",
			want: mikrotikGaugeMetrics(52, 0, 50, 2, 13),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			results, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, results, 1)

			pm := results[0]
			assert.Zero(t, pm.Stats.Errors.Processing.Table)
			for i := range pm.Metrics {
				pm.Metrics[i].Profile = nil
			}
			assertTableMetricsEqual(t, tc.want, pm.Metrics)
		})
	}
}

func mustLoadMikroTikGaugeProfile(t *testing.T) *ddsnmp.Profile {
	t.Helper()

	profile, err := ddsnmp.LoadProfileByName("mikrotik-router")
	require.NoError(t, err)

	profile.Definition.Metadata = nil
	profile.Definition.SysobjectIDMetadata = nil
	profile.Definition.MetricTags = nil
	profile.Definition.StaticTags = nil
	profile.Definition.VirtualMetrics = nil
	profile.Definition.Topology = nil
	profile.Definition.Licensing = nil
	profile.Definition.BGP = nil
	profile.Definition.Metrics = slices.DeleteFunc(profile.Definition.Metrics, func(metric ddprofiledefinition.MetricsConfig) bool {
		return metric.Table.Name != "mtxrHlTable"
	})
	require.Len(t, profile.Definition.Metrics, 1)

	return profile
}

func mikrotikGaugeMetrics(temperature, status, voltage, current, power int64) []ddsnmp.Metric {
	return []ddsnmp.Metric{
		mikrotikGaugeMetric("temperature", "cpu-temperature", "Cel", "Temperature", "Temperature reading", temperature),
		{
			Name:        "mtxrHlSensorValue_sensor_status",
			Description: "Component presence and operational status",
			Family:      "Hardware/Sensor/Presence/Value",
			MetricType:  ddprofiledefinition.ProfileMetricTypeGauge,
			Tags:        map[string]string{"sensor_name": "psu-state"},
			Table:       "mtxrHlTable",
			Value:       status,
			MultiValue: map[string]int64{
				"absent_or_faulty": oldmetrix.Bool(status == 0),
				"present_and_ok":   oldmetrix.Bool(status == 1),
			},
			IsTable: true,
		},
		mikrotikGaugeMetric("voltage", "jack-voltage", "V", "Voltage", "Voltage measurement", voltage),
		mikrotikGaugeMetric("current", "input-current", "A", "Current", "Current draw", current),
		mikrotikGaugeMetric("power", "input-power", "W", "Power", "Power consumption", power),
	}
}

func mikrotikGaugeMetric(kind, sensorName, unit, family, description string, value int64) ddsnmp.Metric {
	return ddsnmp.Metric{
		Name:        "mtxrHlSensorValue_" + kind,
		Description: description,
		Family:      "Hardware/Sensor/" + family + "/Value",
		Unit:        unit,
		MetricType:  ddprofiledefinition.ProfileMetricTypeGauge,
		Tags:        map[string]string{"sensor_name": sensorName},
		Table:       "mtxrHlTable",
		Value:       value,
		IsTable:     true,
	}
}
