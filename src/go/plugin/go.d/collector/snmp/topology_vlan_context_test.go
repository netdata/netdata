// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWithTopologyVLANContextTags_AddsContextFields(t *testing.T) {
	tags := withTopologyVLANContextTags(map[string]string{"key": "value"}, "200", "servers")
	require.Equal(t, "value", tags["key"])
	require.Equal(t, "200", tags[tagTopologyContextVLANID])
	require.Equal(t, "servers", tags[tagTopologyContextVLANName])
}

func TestIsTopologyVLANContextMetric_FiltersSupportedMetrics(t *testing.T) {
	require.True(t, isTopologyVLANContextMetric(metricFdbEntry))
	require.True(t, isTopologyVLANContextMetric(metricStpPortEntry))
	require.False(t, isTopologyVLANContextMetric(metricCdpCacheEntry))
}
