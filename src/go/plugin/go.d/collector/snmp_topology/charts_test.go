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
	raw := New().ChartTemplateYAML()
	collecttest.AssertChartTemplateSchema(t, raw)

	spec, err := charttpl.DecodeYAML([]byte(raw))
	require.NoError(t, err)
	_, err = chartengine.Compile(spec, 1)
	require.NoError(t, err)

	contexts := chartTemplateContexts(spec.Groups)
	assert.Contains(t, contexts, "netdata.go.plugin.collector.snmp_topology.devices")
	assert.Contains(t, contexts, "netdata.go.plugin.collector.snmp_topology.last_refresh")
	assert.Contains(t, contexts, "netdata.go.plugin.collector.snmp_topology.refreshes")
	assert.NotContains(t, contexts, "snmp_topology.devices")
	assert.NotContains(t, contexts, "snmp_topology.links")
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
