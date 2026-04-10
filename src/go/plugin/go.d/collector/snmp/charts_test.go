// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

func TestCollector_AddProfileScalarMetricChart_LabelsIncludeMetricTags(t *testing.T) {
	collr := New()
	collr.Hostname = "192.0.2.1"
	collr.sysInfo = &snmputils.SysInfo{
		Name:   "test-device",
		Vendor: "test-vendor",
	}

	collr.addProfileScalarMetricChart(ddsnmp.Metric{
		Name:  "license.status",
		Value: 1,
		Tags: map[string]string{
			"component":          "vpn",
			"_license_state_raw": "active",
		},
		Profile: &ddsnmp.ProfileMetrics{
			Tags: map[string]string{"profile_tag": "profile_value"},
		},
	})

	chart := collr.Charts().Get("snmp_device_prof_license_status")
	require.NotNil(t, chart)

	assert.Equal(t, map[string]string{
		"address":           "192.0.2.1",
		"component":         "vpn",
		"license_state_raw": "active",
		"profile_tag":       "profile_value",
		"sysName":           "test-device",
		"vendor":            "test-vendor",
	}, chartLabels(chart))
}

func chartLabels(chart *collectorapi.Chart) map[string]string {
	labels := make(map[string]string, len(chart.Labels))
	for _, label := range chart.Labels {
		labels[label.Key] = label.Value
	}
	return labels
}
