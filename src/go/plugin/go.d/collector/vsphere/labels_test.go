// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/match"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
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

	vmChartID := findChartIDByLabelsAndContext(t, createdCharts, "vsphere.vm_cpu_utilization", map[string]string{"id": vm.ID})
	require.Equal(t, "platform", createdCharts[vmChartID].Labels["vsphere_custom_attribute_owner"])
	require.Equal(t, "payments", createdCharts[vmChartID].Labels["vsphere_tag_service"])

	hostChartID := findChartIDByLabelsAndContext(t, createdCharts, "vsphere.host_cpu_utilization", map[string]string{"id": host.ID})
	require.Equal(t, "prod", createdCharts[hostChartID].Labels["vsphere_tag_env"])
	collecttest.AssertChartCoverage(t, collr, collecttest.ChartCoverageExpectation{})
}

func TestClusterNameLabels(t *testing.T) {
	tests := map[string]struct {
		host *rs.Host
		vm   *rs.VM
		want string
	}{
		"standalone host dummy cluster": {
			host: &rs.Host{
				Name: "Host1",
				Hier: rs.HostHierarchy{Cluster: rs.HierarchyValue{
					ID:   "domain-s1",
					Name: "Host1",
				}},
			},
			vm: &rs.VM{
				Hier: rs.VMHierarchy{
					Cluster: rs.HierarchyValue{ID: "domain-s1", Name: "Host1"},
					Host:    rs.HierarchyValue{Name: "Host1"},
				},
			},
			want: "",
		},
		"real cluster with same name as host": {
			host: &rs.Host{
				Name: "Host1",
				Hier: rs.HostHierarchy{Cluster: rs.HierarchyValue{
					ID:   "domain-c1",
					Name: "Host1",
				}},
			},
			vm: &rs.VM{
				Hier: rs.VMHierarchy{
					Cluster: rs.HierarchyValue{ID: "domain-c1", Name: "Host1"},
					Host:    rs.HierarchyValue{Name: "Host1"},
				},
			},
			want: "Host1",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, getHostClusterName(tc.host))
			require.Equal(t, tc.want, getVMClusterName(tc.vm))
		})
	}
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
