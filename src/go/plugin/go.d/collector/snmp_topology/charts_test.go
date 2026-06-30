// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_ChartTemplateYAML(t *testing.T) {
	raw := newTestSNMPTopologyCollector().ChartTemplateYAML()
	collecttest.AssertChartTemplateSchema(t, raw)

	spec, err := charttpl.DecodeYAML([]byte(raw))
	require.NoError(t, err)
	_, err = chartengine.Compile(spec, 1)
	require.NoError(t, err)

	contexts := chartTemplateContexts(spec.Groups)
	for name, tc := range map[string]struct {
		context string
	}{
		"devices":      {context: "netdata.go.plugin.collector.snmp_topology.devices"},
		"last-refresh": {context: "netdata.go.plugin.collector.snmp_topology.last_refresh"},
		"refreshes":    {context: "netdata.go.plugin.collector.snmp_topology.refreshes"},
	} {
		t.Run("contains/"+name, func(t *testing.T) {
			assert.Contains(t, contexts, tc.context)
		})
	}
	for name, tc := range map[string]struct {
		context string
	}{
		"legacy-devices": {context: "snmp_topology.devices"},
		"legacy-links":   {context: "snmp_topology.links"},
	} {
		t.Run("not-contains/"+name, func(t *testing.T) {
			assert.NotContains(t, contexts, tc.context)
		})
	}
}

func chartTemplateContexts(groups []charttpl.Group) map[string]struct{} {
	contexts := make(map[string]struct{})
	for _, group := range groups {
		for _, chart := range group.Charts {
			contexts[chart.Context] = struct{}{}
		}
		for ctx := range chartTemplateContexts(group.Groups) {
			contexts[ctx] = struct{}{}
		}
	}
	return contexts
}
