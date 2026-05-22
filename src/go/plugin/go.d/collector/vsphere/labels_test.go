// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/match"
)

func TestCollector_AddsUserMetadataLabels(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, collr.Init(context.Background()))
	collr.scraper = mockScraper{collr.scraper}
	for _, vm := range collr.resources.VMs {
		vm.Labels = map[string]string{
			"vsphere_custom_attribute_owner": "platform",
			"vsphere_tag_service":            "payments",
		}
	}
	for _, host := range collr.resources.Hosts {
		host.Labels = map[string]string{"vsphere_tag_env": "prod"}
	}

	require.NotEmpty(t, collectScalarSeriesForTest(t, collr))

	vm := firstSortedVM(t, collr)
	host := firstSortedHost(t, collr)
	createdCharts, _ := v2CreatedChartsAndDims(buildV2PlanForTest(t, collr))

	vmChartID := v2ChartTemplateID("vsphere.vm_cpu_utilization") + "_" + vm.ID
	require.Equal(t, "platform", createdCharts[vmChartID].Labels["vsphere_custom_attribute_owner"])
	require.Equal(t, "payments", createdCharts[vmChartID].Labels["vsphere_tag_service"])

	hostChartID := v2ChartTemplateID("vsphere.host_cpu_utilization") + "_" + host.ID
	require.Equal(t, "prod", createdCharts[hostChartID].Labels["vsphere_tag_env"])
}

func TestCollector_Init_ReturnsFalseIfInvalidUserMetadataLabelConfig(t *testing.T) {
	tests := map[string]struct {
		setup func(*Collector)
		want  string
	}{
		"invalid tag category pattern": {
			setup: func(c *Collector) { c.TagCategories = []string{"["} },
			want:  "tag_categories has invalid pattern",
		},
		"empty negative tag category pattern": {
			setup: func(c *Collector) { c.TagCategories = []string{"!"} },
			want:  "tag_categories has invalid empty negative pattern",
		},
		"all-negative custom attribute pattern list": {
			setup: func(c *Collector) { c.CustomAttributes = []string{"!Secret"} },
			want:  "custom_attributes must include at least one positive pattern",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.URL = "https://vcenter.local"
			collr.Username = "user"
			collr.Password = "pass"
			tc.setup(collr)

			require.ErrorContains(t, collr.Init(context.Background()), tc.want)
		})
	}
}

func TestPatternListMatcherPreservesUserMetadataListItems(t *testing.T) {
	m, err := match.NewPatternListMatcher("custom_attributes", []string{"!Business Secret", "Cost Center", "Business*"})
	require.NoError(t, err)

	require.True(t, m.MatchString("Cost Center"))
	require.True(t, m.MatchString("Business Unit"))
	require.False(t, m.MatchString("Cost"))
	require.False(t, m.MatchString("Business Secret"))
}
