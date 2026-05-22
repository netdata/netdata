// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vmware/govmomi/performance"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
	scrapepkg "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/scrape"
)

func TestVSphereMethods(t *testing.T) {
	methods := vsphereMethods()

	require.Len(t, methods, 2)
	byID := make(map[string]funcapi.MethodConfig, len(methods))
	for _, method := range methods {
		byID[method.ID] = method
	}

	tests := map[string]struct {
		wantName     string
		responseType string
		alias        string
		presentation bool
	}{
		"readiness": {
			wantName: "vSphere Readiness",
		},
		"topology:vsphere": {
			wantName:     "vSphere Topology",
			responseType: "topology",
			alias:        "topology:vsphere",
			presentation: true,
		},
	}

	for id, tc := range tests {
		t.Run(id, func(t *testing.T) {
			method := byID[id]
			require.Equal(t, tc.wantName, method.Name)
			require.Equal(t, 30, method.UpdateEvery)
			require.True(t, method.RequireCloud)
			require.False(t, method.AgentWide)
			if tc.alias != "" {
				require.Contains(t, method.Aliases, tc.alias)
			}
			if tc.responseType != "" {
				require.Equal(t, tc.responseType, method.ResponseType)
			}
			if tc.presentation {
				require.NotNil(t, method.Presentation())
			}
		})
	}
}

func TestFuncReadiness_Handle(t *testing.T) {
	tests := map[string]struct {
		method    string
		collector func() *Collector
		want      int
		check     func(*testing.T, map[string][]any)
	}{
		"without discovery": {
			method:    "readiness",
			collector: New,
			want:      200,
			check: func(t *testing.T, rows map[string][]any) {
				require.Equal(t, "not_ready", rows["target_url"][2])
				require.Equal(t, "not_ready", rows["inventory_cache"][2])
				require.Equal(t, "disabled", rows["vsan"][2])
			},
		},
		"partial init state": {
			method: "readiness",
			collector: func() *Collector {
				collr := New()
				collr.URL = "https://vcenter.local"
				collr.Username = "user"
				collr.Password = "[REDACTED_SECRET]"
				return collr
			},
			want: 200,
			check: func(t *testing.T, rows map[string][]any) {
				require.Equal(t, "ok", rows["target_url"][2])
				require.Equal(t, "ok", rows["credentials"][2])
				require.Equal(t, "not_ready", rows["client"][2])
				require.Equal(t, "not_ready", rows["inventory_cache"][2])
			},
		},
		"client row requires vSphere client": {
			method: "readiness",
			collector: func() *Collector {
				collr := New()
				collr.URL = "https://vcenter.local"
				collr.Username = "user"
				collr.Password = "[REDACTED_SECRET]"
				collr.discoverer = readinessDiscoverer{}
				collr.scraper = readinessScraper{}
				return collr
			},
			want: 200,
			check: func(t *testing.T, rows map[string][]any) {
				require.Equal(t, "not_ready", rows["client"][2])
			},
		},
		"with vSAN cached data": {
			method: "readiness",
			collector: func() *Collector {
				collr := newVSANTestCollector(true)
				collr.URL = "https://vcenter.local"
				collr.Username = "user"
				collr.Password = "[REDACTED_SECRET]"
				for _, cluster := range collr.resources.Clusters {
					cluster.MetricList = performance.MetricList{{CounterId: 1}}
				}
				for _, host := range collr.resources.Hosts {
					host.MetricList = performance.MetricList{{CounterId: 1}}
				}
				for _, vm := range collr.resources.VMs {
					vm.MetricList = performance.MetricList{{CounterId: 1}}
				}
				return collr
			},
			want: 200,
			check: func(t *testing.T, rows map[string][]any) {
				require.Equal(t, "ok", rows["target_url"][2])
				require.Equal(t, "ok", rows["credentials"][2])
				require.Equal(t, "ok", rows["inventory_cache"][2])
				require.Equal(t, "ok", rows["vsan"][2])
				require.Contains(t, rows["vsan"][3], "vSAN data cached")
			},
		},
		"with empty vSAN cached data": {
			method: "readiness",
			collector: func() *Collector {
				collr := newVSANTestCollector(true)
				collr.vsanMetrics = &scrapepkg.VSANMetrics{}
				return collr
			},
			want: 200,
			check: func(t *testing.T, rows map[string][]any) {
				require.Equal(t, "warning", rows["vsan"][2])
				require.Contains(t, rows["vsan"][3], "last vSAN scrape returned no data")
			},
		},
		"unknown method": {
			method:    "unknown",
			collector: New,
			want:      404,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			handler := &funcReadiness{collector: tc.collector()}

			resp := handler.Handle(context.Background(), tc.method, nil)

			require.Equal(t, tc.want, resp.Status)
			if tc.check != nil {
				require.NotEmpty(t, resp.Columns)
				tc.check(t, readinessRowsFromResponse(t, resp))
			}
		})
	}
}

func readinessRowsFromResponse(t *testing.T, resp *funcapi.FunctionResponse) map[string][]any {
	t.Helper()

	rows, ok := resp.Data.([][]any)
	require.True(t, ok)
	byCheck := make(map[string][]any, len(rows))
	for _, row := range rows {
		require.Len(t, row, len(readinessColumns))
		check, ok := row[0].(string)
		require.True(t, ok)
		byCheck[check] = row
	}
	return byCheck
}

type readinessDiscoverer struct{}

func (readinessDiscoverer) Discover() (*rs.Resources, error) {
	return nil, nil
}

type readinessScraper struct{}

func (readinessScraper) ScrapeHosts(rs.Hosts) []performance.EntityMetric {
	return nil
}

func (readinessScraper) ScrapeVMs(rs.VMs) []performance.EntityMetric {
	return nil
}

func (readinessScraper) ScrapeDatastores(rs.Datastores) []performance.EntityMetric {
	return nil
}

func (readinessScraper) ScrapeClusters(rs.Clusters) []performance.EntityMetric {
	return nil
}

func (readinessScraper) ScrapeVSAN(rs.Clusters, rs.Hosts, rs.VMs) *scrapepkg.VSANMetrics {
	return nil
}
