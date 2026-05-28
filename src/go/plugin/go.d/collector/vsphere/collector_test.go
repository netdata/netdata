// SPDX-License-Identifier: GPL-3.0-or-later
package vsphere

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware/govmomi/performance"
	"github.com/vmware/govmomi/simulator"
	mo25 "github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/match"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		setup   func(*Collector)
		wantErr string
		check   func(*testing.T, *Collector)
	}{
		"success": {
			check: func(t *testing.T, collr *Collector) {
				assert.NotNil(t, collr.discoverer)
				assert.NotNil(t, collr.scraper)
				assert.NotNil(t, collr.resources)
				assert.NotNil(t, collr.discoveryTask)
				assert.True(t, collr.discoveryTask.isRunning())
			},
		},
		"URL not set": {
			setup:   func(c *Collector) { c.URL = "" },
			wantErr: "url",
		},
		"username not set": {
			setup:   func(c *Collector) { c.Username = "" },
			wantErr: "username",
		},
		"password not set": {
			setup:   func(c *Collector) { c.Password = "" },
			wantErr: "password",
		},
		"discovery interval not positive": {
			setup:   func(c *Collector) { c.DiscoveryInterval = 0 },
			wantErr: "discovery_interval must be greater than zero",
		},
		"wrong TLS CA": {
			setup:   func(c *Collector) { c.ClientConfig.TLSConfig.TLSCA = "testdata/tls" },
			wantErr: "testdata/tls",
		},
		"connection refused": {
			setup:   func(c *Collector) { c.URL = "http://127.0.0.1:32001" },
			wantErr: "connect",
		},
		"invalid host include format": {
			setup:   func(c *Collector) { c.HostsInclude = match.HostIncludes{"invalid"} },
			wantErr: "host_include",
		},
		"invalid VM include format": {
			setup:   func(c *Collector) { c.VMsInclude = match.VMIncludes{"invalid"} },
			wantErr: "vm_include",
		},
		"invalid datastore include format": {
			setup:   func(c *Collector) { c.DatastoresInclude = match.DatastoreIncludes{"invalid"} },
			wantErr: "datastore_include",
		},
		"invalid cluster include format": {
			setup:   func(c *Collector) { c.ClustersInclude = match.ClusterIncludes{"invalid"} },
			wantErr: "cluster_include",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr, _, teardown := prepareVSphereSim(t)
			defer teardown()
			if tc.setup != nil {
				tc.setup(collr)
			}

			err := collr.Init(context.Background())
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			if tc.check != nil {
				tc.check(t, collr)
			}
		})
	}
}

func TestCollector_InitReentrantResetsRuntimeState(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, collr.Init(context.Background()))
	collr.scraper = mockScraper{collr.scraper}
	firstRun := collectScalarSeriesForTest(t, collr)
	require.NotEmpty(t, firstRun)

	var keepHostName, keepHostID string
	for _, host := range collr.resources.Hosts {
		if keepHostID == "" {
			keepHostName = host.Name
			keepHostID = host.ID
		}
	}
	require.NotEmpty(t, keepHostName)

	collr.HostsInclude = match.HostIncludes{"/*/*/" + keepHostName}
	require.NoError(t, collr.Init(context.Background()))
	require.Len(t, collr.resources.Hosts, 1)
	require.NotNil(t, collr.resources.Hosts.Get(keepHostID))

	collr.scraper = mockScraper{collr.scraper}
	secondRun := collectScalarSeriesForTest(t, collr)
	require.NotEmpty(t, secondRun)
	require.True(t, scalarSeriesHasLabel(secondRun, "id", keepHostID))
}

func TestCollector_validateConfig_IgnoresDisabledOptionalSelectors(t *testing.T) {
	collr := New()
	collr.URL = "https://vcenter.local"
	collr.Username = "user"
	collr.Password = "[REDACTED_SECRET]"
	collr.DatastoreClustersInclude = match.DatastoreClusterIncludes{"!*"}
	collr.VSANClustersInclude = match.VSANClusterIncludes{"!*"}
	collr.VSANHostsInclude = match.VSANHostIncludes{"!*"}
	collr.VSANVMsInclude = match.VSANVMIncludes{"!*"}

	require.NoError(t, collr.validateConfig())
}

func TestCollector_validateConfig_ValidatesEnabledOptionalSelectors(t *testing.T) {
	tests := map[string]struct {
		setup func(*Collector)
		want  string
	}{
		"datastore clusters": {
			setup: func(c *Collector) {
				c.CollectDatastoreClusters = true
				c.DatastoreClustersInclude = match.DatastoreClusterIncludes{"!*"}
			},
			want: "datastore_cluster_include must include at least one positive pattern",
		},
		"vSAN clusters": {
			setup: func(c *Collector) {
				c.CollectVSAN = true
				c.VSANClustersInclude = match.VSANClusterIncludes{"!*"}
			},
			want: "vsan_cluster_include must include at least one positive pattern",
		},
		"vSAN hosts": {
			setup: func(c *Collector) {
				c.CollectVSAN = true
				c.VSANHostsInclude = match.VSANHostIncludes{"!*"}
			},
			want: "vsan_host_include must include at least one positive pattern",
		},
		"vSAN VMs": {
			setup: func(c *Collector) {
				c.CollectVSAN = true
				c.VSANVMsInclude = match.VSANVMIncludes{"!*"}
			},
			want: "vsan_vm_include must include at least one positive pattern",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.URL = "https://vcenter.local"
			collr.Username = "user"
			collr.Password = "[REDACTED_SECRET]"
			tc.setup(collr)

			require.ErrorContains(t, collr.validateConfig(), tc.want)
		})
	}
}

func TestCollector_Check(t *testing.T) {
	assert.NoError(t, New().Check(context.Background()))
}

func TestCollector_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func(t *testing.T) (*Collector, func())
		check   func(t *testing.T, collr *Collector)
	}{
		"initialized": {
			prepare: func(t *testing.T) (*Collector, func()) {
				collr, _, teardown := prepareVSphereSim(t)
				require.NoError(t, collr.Init(context.Background()))
				return collr, teardown
			},
			check: func(t *testing.T, collr *Collector) {
				assert.True(t, collr.discoveryTask.isStopped())
				assert.False(t, collr.discoveryTask.isRunning())
				assert.Nil(t, collr.vsClient)
			},
		},
		"not initialized": {
			prepare: func(t *testing.T) (*Collector, func()) {
				return New(), func() {}
			},
			check: func(t *testing.T, collr *Collector) {
				assert.Nil(t, collr.vsClient)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := tc.prepare(t)
			defer cleanup()

			assert.NotPanics(t, func() { collr.Cleanup(context.Background()) })
			tc.check(t, collr)
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, collr.Init(context.Background()))
	collr.scraper = mockScraper{collr.scraper}

	series := collectScalarSeriesForTest(t, collr)
	require.NotEmpty(t, series)

	requireScalarSeriesValue(t, series, "inventory_objects_datacenters", "inventory", 1)
	requireScalarSeriesValue(t, series, "inventory_objects_hosts", "inventory", 4)
	requireScalarSeriesValue(t, series, "inventory_objects_vms", "inventory", 4)
	requireScalarSeriesValue(t, series, "inventory_objects_datastores", "inventory", 1)
	requireScalarSeriesValue(t, series, "inventory_objects_resource_pools", "inventory", 1)

	requireScalarSeriesValue(t, series, "host_cpu_utilization_used", "host-21", 100)
	requireScalarSeriesValue(t, series, "host_disk_max_latency_latency", "host-21", 100)
	requireScalarSeriesValue(t, series, "host_net_errors_received", "host-21", 100)
	requireScalarSeriesValue(t, series, "host_overall_status_gray", "host-21", 1)
	requireScalarSeriesValue(t, series, "host_power_state_powered_on", "host-21", 1)
	requireScalarSeriesValue(t, series, "host_connection_state_connected", "host-21", 1)
	requireScalarSeriesValue(t, series, "host_maintenance_status_normal", "host-21", 1)
	requireScalarSeriesValue(t, series, "host_system_uptime_uptime", "host-21", 100)

	requireScalarSeriesValue(t, series, "vm_cpu_utilization_used", "vm-62", 200)
	requireScalarSeriesValue(t, series, "vm_disk_max_latency_latency", "vm-62", 200)
	requireScalarSeriesValue(t, series, "vm_net_drops_received", "vm-62", 200)
	requireScalarSeriesValue(t, series, "vm_overall_status_green", "vm-62", 1)
	requireScalarSeriesValue(t, series, "vm_power_state_powered_on", "vm-62", 1)
	requireScalarSeriesValue(t, series, "vm_tools_running_status_unknown", "vm-62", 1)
	requireScalarSeriesValue(t, series, "vm_tools_version_status_unknown", "vm-62", 1)
	requireScalarSeriesValue(t, series, "vm_config_cpu_vcpus", "vm-62", 1)
	requireScalarSeriesValue(t, series, "vm_config_devices_disks", "vm-62", 1)
	requireScalarSeriesValue(t, series, "vm_storage_usage_uncommitted", "vm-62", 10737418240)
	requireScalarSeriesValue(t, series, "vm_snapshot_count_count", "vm-62", 0)

	requireScalarSeriesValue(t, series, "datastore_space_usage_capacity", "datastore-59", 4398046511104)
	requireScalarSeriesValue(t, series, "datastore_space_usage_used", "datastore-59", 42949672960)
	requireScalarSeriesValue(t, series, "datastore_space_utilization_used", "datastore-59", 97)
	requireScalarSeriesValue(t, series, "datastore_overall_status_green", "datastore-59", 1)
	requireScalarSeriesValue(t, series, "datastore_accessibility_status_accessible", "datastore-59", 1)
	requireScalarSeriesValue(t, series, "datastore_maintenance_status_normal", "datastore-59", 1)
	requireScalarSeriesValue(t, series, "datastore_multiple_host_access_unknown", "datastore-59", 1)
	requireScalarSeriesValue(t, series, "datastore_disk_iops_reads", "datastore-59", 300)

	requireScalarSeriesValue(t, series, "cluster_hosts_total", "domain-c28", 3)
	requireScalarSeriesValue(t, series, "cluster_cpu_capacity_total", "domain-c28", 6882)
	requireScalarSeriesValue(t, series, "cluster_drs_config_enabled", "domain-c28", 1)
	requireScalarSeriesValue(t, series, "cluster_drs_mode_unknown", "domain-c28", 1)
	requireScalarSeriesValue(t, series, "cluster_ha_host_monitoring_unknown", "domain-c28", 1)
	requireScalarSeriesValue(t, series, "cluster_overall_status_green", "domain-c28", 1)
	requireScalarSeriesValue(t, series, "cluster_cpu_utilization_used", "domain-c28", 400)
	requireScalarSeriesValue(t, series, "cluster_services_fairness_cpu", "domain-c28", 400)
	requireScalarSeriesValue(t, series, "cluster_vm_migrations_vmotion", "domain-c28", 400)

	requireScalarSeriesValue(t, series, "resource_pool_cpu_usage_usage", "resgroup-27", 0)
	requireScalarSeriesValue(t, series, "resource_pool_cpu_allocation_max_usage", "resgroup-27", 4121)
	requireScalarSeriesValue(t, series, "resource_pool_mem_allocation_max_usage", "resgroup-27", 1007681536)
	requireScalarSeriesValue(t, series, "resource_pool_cpu_config_reservation", "resgroup-27", 4121)
	requireScalarSeriesValue(t, series, "resource_pool_mem_config_limit", "resgroup-27", 961)
	requireScalarSeriesValue(t, series, "resource_pool_overall_status_green", "resgroup-27", 1)

	collecttest.AssertChartCoverage(t, collr, collecttest.ChartCoverageExpectation{})
}

func TestCollector_V2CompatibilitySurface(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, collr.Init(context.Background()))
	collr.scraper = mockScraper{collr.scraper}

	require.NotEmpty(t, collectScalarSeriesForTest(t, collr))

	plan := buildV2PlanForTest(t, collr)
	createdCharts, createdDims := v2CreatedChartsAndDims(plan)
	require.NotEmpty(t, createdCharts)

	for chartID, chart := range createdCharts {
		require.NotEmpty(t, chart.Labels["id"], "chart %s must have the V2 instance id label", chartID)
		require.NotEmpty(t, createdDims[chartID], "chart %s must have dimensions", chartID)
	}
	collecttest.AssertChartCoverage(t, collr, collecttest.ChartCoverageExpectation{})
}

func TestSnapshotMaxAgeSeconds(t *testing.T) {
	now := time.Now()
	tests := map[string]struct {
		created time.Time
		want    int64
		delta   float64
	}{
		"zero time": {
			created: time.Time{},
		},
		"future time": {
			created: now.Add(time.Hour),
		},
		"past time": {
			created: now.Add(-2 * time.Hour),
			want:    7200,
			delta:   1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if tc.delta > 0 {
				assert.InDelta(t, tc.want, snapshotMaxAgeSeconds(tc.created), tc.delta)
				return
			}
			assert.EqualValues(t, tc.want, snapshotMaxAgeSeconds(tc.created))
		})
	}
}

func TestCollectInventory(t *testing.T) {
	collr := New()
	collr.resources = &rs.Resources{
		DataCenters:   rs.DataCenters{"dc-1": &rs.Datacenter{}},
		Folders:       rs.Folders{"folder-1": &rs.Folder{}, "folder-2": &rs.Folder{}},
		Clusters:      rs.Clusters{"domain-c1": &rs.Cluster{}},
		Hosts:         rs.Hosts{"host-1": &rs.Host{}, "host-2": &rs.Host{}},
		VMs:           rs.VMs{"vm-1": &rs.VM{}, "vm-2": &rs.VM{}, "vm-3": &rs.VM{}},
		Datastores:    rs.Datastores{"datastore-1": &rs.Datastore{}},
		ResourcePools: rs.ResourcePools{"resgroup-1": &rs.ResourcePool{}},
	}

	series := runMetricWriteForTest(t, collr, collr.collectInventory)

	requireScalarSeriesValue(t, series, "inventory_objects_datacenters", "inventory", 1)
	requireScalarSeriesValue(t, series, "inventory_objects_folders", "inventory", 2)
	requireScalarSeriesValue(t, series, "inventory_objects_clusters", "inventory", 1)
	requireScalarSeriesValue(t, series, "inventory_objects_hosts", "inventory", 2)
	requireScalarSeriesValue(t, series, "inventory_objects_vms", "inventory", 3)
	requireScalarSeriesValue(t, series, "inventory_objects_datastores", "inventory", 1)
	requireScalarSeriesValue(t, series, "inventory_objects_resource_pools", "inventory", 1)
}

func TestCollector_Collect_NonPoweredResourcePropertyOnly(t *testing.T) {
	tests := map[string]struct {
		setup   func(*Collector)
		collect func(*Collector) error
		want    map[string]int64
		missing []string
	}{
		"host": {
			setup: func(c *Collector) {
				c.resources = &rs.Resources{
					Hosts: rs.Hosts{
						"host-1": &rs.Host{
							ID:            "host-1",
							PowerState:    string(types.HostSystemPowerStatePoweredOff),
							OverallStatus: "gray",
						},
					},
				}
			},
			collect: (*Collector).collectHosts,
			want: map[string]int64{
				"host_overall_status_gray":        1,
				"host_power_state_powered_off":    1,
				"host_power_state_powered_on":     0,
				"host_connection_state_connected": 0,
			},
			missing: []string{"host_cpu_utilization_used"},
		},
		"VM": {
			setup: func(c *Collector) {
				c.resources = &rs.Resources{
					VMs: rs.VMs{
						"vm-1": &rs.VM{
							ID:                    "vm-1",
							PowerState:            string(types.VirtualMachinePowerStateSuspended),
							OverallStatus:         "yellow",
							SnapshotCount:         2,
							SnapshotMaxChainDepth: 1,
						},
					},
				}
			},
			collect: (*Collector).collectVMs,
			want: map[string]int64{
				"vm_overall_status_yellow":          1,
				"vm_power_state_suspended":          1,
				"vm_power_state_powered_on":         0,
				"vm_snapshot_count_count":           2,
				"vm_snapshot_max_chain_depth_depth": 1,
			},
			missing: []string{"vm_cpu_utilization_used"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			tc.setup(collr)

			series := runMetricCollectForTest(t, collr, func() error { return tc.collect(collr) })
			for metric, want := range tc.want {
				requireScalarSeriesValue(t, series, metric, firstResourceID(collr), want)
			}
			for _, metric := range tc.missing {
				requireNoScalarSeries(t, series, metric, firstResourceID(collr))
			}
		})
	}
}

func TestCollector_Collect_NoPerfDataKeepsPropertyMetrics(t *testing.T) {
	tests := map[string]struct {
		setup func(*Collector)
		check func(*testing.T, *Collector, map[string]metrix.SampleValue)
	}{
		"hosts": {
			setup: func(c *Collector) { c.scraper = mockScraperNoHostPerf{mockScraper{c.scraper}} },
			check: func(t *testing.T, collr *Collector, series map[string]metrix.SampleValue) {
				for _, host := range collr.resources.Hosts {
					requireScalarSeries(t, series, "host_overall_status_green", host.ID)
					requireScalarSeries(t, series, "host_power_state_powered_on", host.ID)
					requireNoScalarSeries(t, series, "host_cpu_utilization_used", host.ID)
				}
				for _, vm := range collr.resources.VMs {
					requireScalarSeries(t, series, "vm_cpu_utilization_used", vm.ID)
				}
			},
		},
		"VMs": {
			setup: func(c *Collector) { c.scraper = mockScraperNoVMPerf{mockScraper{c.scraper}} },
			check: func(t *testing.T, collr *Collector, series map[string]metrix.SampleValue) {
				for _, host := range collr.resources.Hosts {
					requireScalarSeries(t, series, "host_cpu_utilization_used", host.ID)
				}
				for _, vm := range collr.resources.VMs {
					requireScalarSeries(t, series, "vm_overall_status_green", vm.ID)
					requireScalarSeries(t, series, "vm_power_state_powered_on", vm.ID)
					requireNoScalarSeries(t, series, "vm_cpu_utilization_used", vm.ID)
				}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr, _, teardown := prepareVSphereSim(t)
			defer teardown()

			require.NoError(t, collr.Init(context.Background()))
			tc.setup(collr)

			series := collectScalarSeriesForTest(t, collr)
			require.NotNil(t, series)
			tc.check(t, collr, series)
		})
	}
}

func TestCollector_Collect_PropertyRefreshFailureSkipsStalePropertyMetrics(t *testing.T) {
	tests := map[string]struct {
		setup func(*Collector)
		check func(*testing.T, *Collector, map[string]metrix.SampleValue)
	}{
		"datastores": {
			setup: func(c *Collector) { c.dsPropertyCollector = nil },
			check: func(t *testing.T, collr *Collector, series map[string]metrix.SampleValue) {
				for _, ds := range collr.resources.Datastores {
					requireNoScalarSeries(t, series, "datastore_space_usage_capacity", ds.ID)
					requireNoScalarSeries(t, series, "datastore_overall_status_green", ds.ID)
					requireScalarSeries(t, series, "datastore_disk_io_read", ds.ID)
				}
			},
		},
		"clusters": {
			setup: func(c *Collector) { c.clusterPropertyCollector = nil },
			check: func(t *testing.T, collr *Collector, series map[string]metrix.SampleValue) {
				for _, cl := range collr.resources.Clusters {
					requireNoScalarSeries(t, series, "cluster_hosts_total", cl.ID)
					requireNoScalarSeries(t, series, "cluster_overall_status_green", cl.ID)
					requireScalarSeries(t, series, "cluster_cpu_utilization_used", cl.ID)
				}
			},
		},
		"resource pools": {
			setup: func(c *Collector) { c.rpPropertyCollector = nil },
			check: func(t *testing.T, collr *Collector, series map[string]metrix.SampleValue) {
				for _, rp := range collr.resources.ResourcePools {
					requireNoScalarSeries(t, series, "resource_pool_cpu_usage_usage", rp.ID)
					requireNoScalarSeries(t, series, "resource_pool_overall_status_green", rp.ID)
				}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr, _, teardown := prepareVSphereSim(t)
			defer teardown()

			require.NoError(t, collr.Init(context.Background()))
			collr.scraper = mockScraper{collr.scraper}
			tc.setup(collr)

			series := collectScalarSeriesForTest(t, collr)
			require.NotNil(t, series)
			tc.check(t, collr, series)
		})
	}
}

func TestCollector_Collect_NoPerfDataWarningIsRateLimited(t *testing.T) {
	tests := map[string]struct {
		setup   func(*Collector)
		collect func(*Collector) error
		wantLog string
	}{
		"hosts": {
			setup: func(c *Collector) {
				c.scraper = mockScraperNoHostPerf{}
				c.resources = &rs.Resources{
					Hosts: rs.Hosts{
						"host-1": &rs.Host{
							ID:         "host-1",
							PowerState: string(types.HostSystemPowerStatePoweredOn),
						},
					},
				}
			},
			collect: (*Collector).collectHosts,
			wantLog: "collect host performance metrics",
		},
		"VMs": {
			setup: func(c *Collector) {
				c.scraper = mockScraperNoVMPerf{}
				c.resources = &rs.Resources{
					VMs: rs.VMs{
						"vm-1": &rs.VM{
							ID:         "vm-1",
							PowerState: string(types.VirtualMachinePowerStatePoweredOn),
						},
					},
				}
			},
			collect: (*Collector).collectVMs,
			wantLog: "collect VM performance metrics",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			collr := New()
			collr.Logger = logger.NewWithWriter(&buf)
			tc.setup(collr)

			runMetricCollectForTest(t, collr, func() error { return tc.collect(collr) })
			runMetricCollectForTest(t, collr, func() error { return tc.collect(collr) })

			assert.Equal(t, 1, strings.Count(buf.String(), tc.wantLog))
		})
	}
}

func TestWriteHostPropertyMetrics_RuntimeStatus(t *testing.T) {
	host := &rs.Host{
		ID:                "host-1",
		ConnectionState:   string(types.HostSystemConnectionStateNotResponding),
		PowerState:        string(types.HostSystemPowerStatePoweredOn),
		InMaintenanceMode: true,
		OverallStatus:     "yellow",
	}
	collr := New()

	series := runMetricWriteForTest(t, collr, func() { collr.writeHostPropertyMetrics(host) })

	requireScalarSeriesValue(t, series, "host_connection_state_not_responding", host.ID, 1)
	requireScalarSeriesValue(t, series, "host_connection_state_connected", host.ID, 0)
	requireScalarSeriesValue(t, series, "host_maintenance_status_in_maintenance", host.ID, 1)
	requireScalarSeriesValue(t, series, "host_maintenance_status_normal", host.ID, 0)
}

func TestWriteVMPropertyMetrics_StatusConfigAndStorage(t *testing.T) {
	vm := &rs.VM{
		ID:                  "vm-1",
		ConnectionState:     string(types.VirtualMachineConnectionStateInaccessible),
		PowerState:          string(types.VirtualMachinePowerStatePoweredOn),
		ToolsRunningStatus:  string(types.VirtualMachineToolsRunningStatusGuestToolsRunning),
		ToolsVersionStatus:  string(types.VirtualMachineToolsVersionStatusGuestToolsTooOld),
		ConsolidationNeeded: true,
		ConfigCPU:           4,
		ConfigMemory:        8192,
		ConfigDisks:         2,
		ConfigNICs:          3,
		StorageCommitted:    100,
		StorageUncommitted:  200,
		StorageUnshared:     300,
		OverallStatus:       "green",
	}
	collr := New()

	series := runMetricWriteForTest(t, collr, func() { collr.writeVMPropertyMetrics(vm) })

	requireScalarSeriesValue(t, series, "vm_connection_state_inaccessible", vm.ID, 1)
	requireScalarSeriesValue(t, series, "vm_connection_state_connected", vm.ID, 0)
	requireScalarSeriesValue(t, series, "vm_tools_running_status_running", vm.ID, 1)
	requireScalarSeriesValue(t, series, "vm_tools_running_status_unknown", vm.ID, 0)
	requireScalarSeriesValue(t, series, "vm_tools_version_status_too_old", vm.ID, 1)
	requireScalarSeriesValue(t, series, "vm_tools_version_status_unknown", vm.ID, 0)
	requireScalarSeriesValue(t, series, "vm_consolidation_needed_needed", vm.ID, 1)
	requireScalarSeriesValue(t, series, "vm_consolidation_needed_not_needed", vm.ID, 0)
	requireScalarSeriesValue(t, series, "vm_config_cpu_vcpus", vm.ID, 4)
	requireScalarSeriesValue(t, series, "vm_config_memory_memory", vm.ID, 8192)
	requireScalarSeriesValue(t, series, "vm_config_devices_disks", vm.ID, 2)
	requireScalarSeriesValue(t, series, "vm_config_devices_nics", vm.ID, 3)
	requireScalarSeriesValue(t, series, "vm_storage_usage_committed", vm.ID, 100)
	requireScalarSeriesValue(t, series, "vm_storage_usage_uncommitted", vm.ID, 200)
	requireScalarSeriesValue(t, series, "vm_storage_usage_unshared", vm.ID, 300)
}

func TestWriteClusterPropertyMetrics_ConfigStates(t *testing.T) {
	cluster := &rs.Cluster{
		ID:                      "domain-c1",
		DrsEnabled:              true,
		DrsMode:                 string(types.DrsBehaviorFullyAutomated),
		DrsVmotionRate:          3,
		HaEnabled:               true,
		HaAdmCtrlEnabled:        true,
		HaHostMonitoring:        string(types.ClusterDasConfigInfoServiceStateEnabled),
		HaVMMonitoring:          string(types.ClusterDasConfigInfoVmMonitoringStateVmAndAppMonitoring),
		HaVMComponentProtection: string(types.ClusterDasConfigInfoServiceStateDisabled),
		OverallStatus:           "green",
	}
	collr := New()

	series := runMetricWriteForTest(t, collr, func() { collr.writeClusterPropertyMetrics(cluster) })

	requireScalarSeriesValue(t, series, "cluster_drs_config_enabled", cluster.ID, 1)
	requireScalarSeriesValue(t, series, "cluster_drs_mode_fully_automated", cluster.ID, 1)
	requireScalarSeriesValue(t, series, "cluster_drs_mode_unknown", cluster.ID, 0)
	requireScalarSeriesValue(t, series, "cluster_drs_vmotion_rate_rate", cluster.ID, 3)
	requireScalarSeriesValue(t, series, "cluster_ha_config_enabled", cluster.ID, 1)
	requireScalarSeriesValue(t, series, "cluster_ha_config_admission_control", cluster.ID, 1)
	requireScalarSeriesValue(t, series, "cluster_ha_host_monitoring_enabled", cluster.ID, 1)
	requireScalarSeriesValue(t, series, "cluster_ha_host_monitoring_unknown", cluster.ID, 0)
	requireScalarSeriesValue(t, series, "cluster_ha_vm_monitoring_vm_and_app_monitoring", cluster.ID, 1)
	requireScalarSeriesValue(t, series, "cluster_ha_vm_monitoring_unknown", cluster.ID, 0)
	requireScalarSeriesValue(t, series, "cluster_ha_vm_component_protection_disabled", cluster.ID, 1)
	requireScalarSeriesValue(t, series, "cluster_ha_vm_component_protection_unknown", cluster.ID, 0)
}

func TestUpdateResourcePoolFromProperties_ZeroValueOptionalProperties(t *testing.T) {
	rp := &rs.ResourcePool{
		ID:                           "resgroup-1",
		OverallCpuUsage:              1,
		OverallCpuDemand:             2,
		GuestMemoryUsage:             3,
		HostMemoryUsage:              4,
		DistributedCpuEntitlement:    5,
		DistributedMemoryEntitlement: 6,
		PrivateMemory:                7,
		SharedMemory:                 8,
		SwappedMemory:                9,
		BalloonedMemory:              10,
		OverheadMemory:               11,
		ConsumedOverheadMemory:       12,
		CompressedMemory:             13,
		CpuReservationUsed:           14,
		CpuMaxUsage:                  15,
		CpuUnreservedForVm:           16,
		MemReservationUsed:           17,
		MemMaxUsage:                  18,
		MemUnreservedForVm:           19,
		CpuReservation:               20,
		CpuLimit:                     21,
		MemReservation:               22,
		MemLimit:                     23,
	}

	require.NotPanics(t, func() {
		updateResourcePoolFromProperties(rp, mo25.ResourcePool{})
	})

	assert.Zero(t, rp.OverallCpuUsage)
	assert.Zero(t, rp.GuestMemoryUsage)
	assert.Zero(t, rp.CpuReservationUsed)
	assert.Zero(t, rp.MemReservationUsed)
	assert.Zero(t, rp.CpuReservation)
	assert.EqualValues(t, -1, rp.CpuLimit)
	assert.Zero(t, rp.MemReservation)
	assert.EqualValues(t, -1, rp.MemLimit)
}

func collectScalarSeriesForTest(t *testing.T, collr *Collector) map[string]metrix.SampleValue {
	t.Helper()

	mx, err := collecttest.CollectScalarSeries(collr, metrix.ReadRaw())
	require.NoError(t, err)
	if len(mx) == 0 {
		return nil
	}
	return mx
}

func buildV2PlanForTest(t *testing.T, collr *Collector) chartengine.Plan {
	t.Helper()

	engine, err := chartengine.New()
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(collr.ChartTemplateYAML()), 1))

	reader := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	attempt, err := engine.PreparePlan(reader)
	require.NoError(t, err)
	defer attempt.Abort()

	plan := attempt.Plan()
	require.NoError(t, attempt.Commit())
	return plan
}

func requireChartSelectorsMatchSeries(t *testing.T, collr *Collector, contextPrefixes ...string) {
	t.Helper()

	spec, err := charttpl.DecodeYAML([]byte(collr.ChartTemplateYAML()))
	require.NoError(t, err)

	reader := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	require.NotZero(t, requireChartGroupSelectorsMatchSeries(t, reader, spec.Groups, contextParts(spec.ContextNamespace), contextPrefixes))
}

func requireChartGroupSelectorsMatchSeries(t *testing.T, reader metrix.Reader, groups []charttpl.Group, parent, contextPrefixes []string) int {
	t.Helper()

	matchedContexts := 0
	for _, group := range groups {
		parts := append(append([]string(nil), parent...), contextParts(group.ContextNamespace)...)
		for _, chart := range group.Charts {
			contextName := strings.Join(append(append([]string(nil), parts...), strings.TrimSpace(chart.Context)), ".")
			if !matchesAnyContextPrefix(contextName, contextPrefixes) {
				continue
			}
			matchedContexts++
			for _, dim := range chart.Dimensions {
				requireSelectorMatchesSeries(t, reader, contextName, dim.Selector)
			}
		}
		matchedContexts += requireChartGroupSelectorsMatchSeries(t, reader, group.Groups, parts, contextPrefixes)
	}
	return matchedContexts
}

func matchesAnyContextPrefix(contextName string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if contextName == prefix || strings.HasPrefix(contextName, prefix) {
			return true
		}
	}
	return false
}

func requireSelectorMatchesSeries(t *testing.T, reader metrix.Reader, contextName, selector string) {
	t.Helper()

	sel, err := metrixselector.Parse(selector)
	require.NoErrorf(t, err, "chart context %s selector %q", contextName, selector)

	matched := false
	reader.ForEachSeriesIdentity(func(_ metrix.SeriesIdentity, _ metrix.SeriesMeta, metricName string, labels metrix.LabelView, _ metrix.SampleValue) {
		if sel.Matches(metricName, labels) {
			matched = true
		}
	})
	require.Truef(t, matched, "chart context %s selector %q matches no series", contextName, selector)
}

func contextParts(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return []string{value}
}

func v2CreatedChartsAndDims(plan chartengine.Plan) (map[string]chartengine.CreateChartAction, map[string]map[string]chartengine.CreateDimensionAction) {
	charts := make(map[string]chartengine.CreateChartAction)
	dims := make(map[string]map[string]chartengine.CreateDimensionAction)
	for _, action := range plan.Actions {
		switch v := action.(type) {
		case chartengine.CreateChartAction:
			charts[v.ChartID] = v
		case chartengine.CreateDimensionAction:
			if _, ok := dims[v.ChartID]; !ok {
				dims[v.ChartID] = make(map[string]chartengine.CreateDimensionAction)
			}
			dims[v.ChartID][v.Name] = v
		}
	}
	return charts, dims
}

func scalarSeriesHasLabel(series map[string]metrix.SampleValue, key, value string) bool {
	needle := fmt.Sprintf(`%s="%s"`, key, value)
	for name := range series {
		if strings.Contains(name, needle) {
			return true
		}
	}
	return false
}

func runMetricWriteForTest(t *testing.T, collr *Collector, write func()) map[string]metrix.SampleValue {
	t.Helper()

	cycle := mustCycleController(t, collr.MetricStore())
	cycle.BeginCycle()
	write()
	require.NoError(t, cycle.CommitCycleSuccess())
	return scalarSeriesFromReaderForTest(collr.MetricStore().Read(metrix.ReadRaw()))
}

func runMetricCollectForTest(t *testing.T, collr *Collector, collect func() error) map[string]metrix.SampleValue {
	t.Helper()

	cycle := mustCycleController(t, collr.MetricStore())
	cycle.BeginCycle()
	if err := collect(); err != nil {
		cycle.AbortCycle()
		require.NoError(t, err)
	}
	require.NoError(t, cycle.CommitCycleSuccess())
	return scalarSeriesFromReaderForTest(collr.MetricStore().Read(metrix.ReadRaw()))
}

func scalarSeriesFromReaderForTest(reader metrix.Reader) map[string]metrix.SampleValue {
	out := make(map[string]metrix.SampleValue)
	reader.ForEachSeries(func(name string, labels metrix.LabelView, value metrix.SampleValue) {
		out[scalarSeriesKeyForTest(name, labels)] = value
	})
	return out
}

func scalarSeriesKeyForTest(name string, labels metrix.LabelView) string {
	if labels == nil || labels.Len() == 0 {
		return name
	}

	var b strings.Builder
	b.WriteString(name)
	b.WriteByte('{')
	first := true
	labels.Range(func(key, value string) bool {
		if !first {
			b.WriteByte(',')
		}
		first = false
		b.WriteString(key)
		b.WriteString(`="`)
		b.WriteString(value)
		b.WriteByte('"')
		return true
	})
	b.WriteByte('}')
	return b.String()
}

func requireScalarSeries(t *testing.T, series map[string]metrix.SampleValue, metric, id string) {
	t.Helper()

	_, ok := findScalarSeries(series, metric, id)
	require.Truef(t, ok, "expected metric %s with id=%s in %v", metric, id, scalarSeriesKeys(series))
}

func requireNoScalarSeries(t *testing.T, series map[string]metrix.SampleValue, metric, id string) {
	t.Helper()

	_, ok := findScalarSeries(series, metric, id)
	require.Falsef(t, ok, "unexpected metric %s with id=%s", metric, id)
}

func requireScalarSeriesValue(t *testing.T, series map[string]metrix.SampleValue, metric, id string, want int64) {
	t.Helper()

	got, ok := findScalarSeries(series, metric, id)
	require.Truef(t, ok, "expected metric %s with id=%s in %v", metric, id, scalarSeriesKeys(series))
	require.EqualValues(t, want, got)
}

func findScalarSeries(series map[string]metrix.SampleValue, metric, id string) (metrix.SampleValue, bool) {
	prefix := metric + "{"
	idLabel := fmt.Sprintf(`id="%s"`, id)
	for key, value := range series {
		if (key == metric || strings.HasPrefix(key, prefix)) && strings.Contains(key, idLabel) {
			return value, true
		}
	}
	return 0, false
}

func scalarSeriesKeys(series map[string]metrix.SampleValue) []string {
	keys := make([]string, 0, len(series))
	for key := range series {
		keys = append(keys, key)
	}
	return keys
}

func firstResourceID(collr *Collector) string {
	for id := range collr.resources.Hosts {
		return id
	}
	for id := range collr.resources.VMs {
		return id
	}
	return ""
}

func mustCycleController(t *testing.T, store metrix.CollectorStore) metrix.CycleController {
	t.Helper()

	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok)
	return managed.CycleController()
}

func TestCollector_Collect_Run(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()

	collr.DiscoveryInterval = confopt.Duration(time.Second * 2)
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	runs := 20
	for i := range runs {
		assert.True(t, len(collectScalarSeriesForTest(t, collr)) > 0)
		if i < 6 {
			time.Sleep(time.Second)
		}
	}
	collecttest.AssertChartCoverage(t, collr, collecttest.ChartCoverageExpectation{})
}

func prepareVSphereSim(t *testing.T) (collr *Collector, model *simulator.Model, teardown func()) {
	model, srv := createSim(t)
	collr = New()
	teardown = func() { model.Remove(); srv.Close(); collr.Cleanup(context.Background()) }

	collr.Username = "administrator"
	collr.Password = "password"
	collr.URL = srv.URL.String()
	collr.TLSConfig.InsecureSkipVerify = true

	return collr, model, teardown
}

func createSim(t *testing.T) (*simulator.Model, *simulator.Server) {
	model := simulator.VPX()
	err := model.Create()
	require.NoError(t, err)
	model.Service.TLS = new(tls.Config)
	return model, model.Service.NewServer()
}

type mockScraper struct {
	scraper
}

func (s mockScraper) ScrapeHosts(hosts rs.Hosts) []performance.EntityMetric {
	ms := s.scraper.ScrapeHosts(hosts)
	return populateMetrics(ms, 100)
}
func (s mockScraper) ScrapeVMs(vms rs.VMs) []performance.EntityMetric {
	ms := s.scraper.ScrapeVMs(vms)
	return populateMetrics(ms, 200)
}
func (s mockScraper) ScrapeDatastores(datastores rs.Datastores) []performance.EntityMetric {
	ms := s.scraper.ScrapeDatastores(datastores)
	return populateMetrics(ms, 300)
}
func (s mockScraper) ScrapeClusters(clusters rs.Clusters) []performance.EntityMetric {
	ms := s.scraper.ScrapeClusters(clusters)
	return populateMetrics(ms, 400)
}

func populateMetrics(ms []performance.EntityMetric, value int64) []performance.EntityMetric {
	for i := range ms {
		for ii := range ms[i].Value {
			v := &ms[i].Value[ii].Value
			if *v == nil {
				*v = append(*v, value)
			} else {
				(*v)[0] = value
			}
		}
	}
	return ms
}

type mockHostMatcher struct{ name string }
type mockVMMatcher struct{ name string }

func (m mockHostMatcher) Match(host *rs.Host) bool { return m.name == host.ID }
func (m mockVMMatcher) Match(vm *rs.VM) bool       { return m.name == vm.ID }

type mockScraperNoHostPerf struct {
	mockScraper
}

func (s mockScraperNoHostPerf) ScrapeHosts(rs.Hosts) []performance.EntityMetric {
	return nil
}

type mockScraperNoVMPerf struct {
	mockScraper
}

func (s mockScraperNoVMPerf) ScrapeVMs(rs.VMs) []performance.EntityMetric {
	return nil
}

// mockScraperNoDSPerf wraps a scraper but returns no perf data for datastores (simulates vSAN).
type mockScraperNoDSPerf struct {
	scraper
}

func (s mockScraperNoDSPerf) ScrapeHosts(hosts rs.Hosts) []performance.EntityMetric {
	return populateMetrics(s.scraper.ScrapeHosts(hosts), 100)
}
func (s mockScraperNoDSPerf) ScrapeVMs(vms rs.VMs) []performance.EntityMetric {
	return populateMetrics(s.scraper.ScrapeVMs(vms), 200)
}
func (s mockScraperNoDSPerf) ScrapeDatastores(_ rs.Datastores) []performance.EntityMetric {
	return nil
}
func (s mockScraperNoDSPerf) ScrapeClusters(clusters rs.Clusters) []performance.EntityMetric {
	return populateMetrics(s.scraper.ScrapeClusters(clusters), 400)
}

// mockScraperNoClusterPerf wraps a scraper but returns no perf data for clusters.
type mockScraperNoClusterPerf struct {
	scraper
}

func (s mockScraperNoClusterPerf) ScrapeHosts(hosts rs.Hosts) []performance.EntityMetric {
	return populateMetrics(s.scraper.ScrapeHosts(hosts), 100)
}
func (s mockScraperNoClusterPerf) ScrapeVMs(vms rs.VMs) []performance.EntityMetric {
	return populateMetrics(s.scraper.ScrapeVMs(vms), 200)
}
func (s mockScraperNoClusterPerf) ScrapeDatastores(datastores rs.Datastores) []performance.EntityMetric {
	return populateMetrics(s.scraper.ScrapeDatastores(datastores), 300)
}
func (s mockScraperNoClusterPerf) ScrapeClusters(_ rs.Clusters) []performance.EntityMetric {
	return nil
}

func TestCollector_Collect_NoPerfData(t *testing.T) {
	tests := map[string]struct {
		setNoPerfScraper func(*Collector)
		restoreScraper   func(*Collector)
		checkNoPerf      func(*testing.T, *Collector, map[string]metrix.SampleValue, simulator.Model)
		checkWithPerf    func(*testing.T, *Collector, map[string]metrix.SampleValue, simulator.Model)
	}{
		"datastores": {
			setNoPerfScraper: func(c *Collector) { c.scraper = mockScraperNoDSPerf{c.scraper} },
			restoreScraper:   func(c *Collector) { c.scraper = mockScraper{c.scraper.(mockScraperNoDSPerf).scraper} },
			checkNoPerf: func(t *testing.T, collr *Collector, series map[string]metrix.SampleValue, count simulator.Model) {
				for _, ds := range collr.resources.Datastores {
					requireScalarSeries(t, series, "datastore_space_usage_capacity", ds.ID)
					requireScalarSeries(t, series, "datastore_space_usage_free", ds.ID)
					requireScalarSeries(t, series, "datastore_space_usage_used", ds.ID)
					requireScalarSeries(t, series, "datastore_space_utilization_used", ds.ID)
					requireScalarSeries(t, series, "datastore_space_usage_uncommitted", ds.ID)
					requireScalarSeries(t, series, "datastore_overall_status_green", ds.ID)
					requireScalarSeries(t, series, "datastore_accessibility_status_accessible", ds.ID)
					requireScalarSeries(t, series, "datastore_maintenance_status_normal", ds.ID)
					requireScalarSeries(t, series, "datastore_multiple_host_access_unknown", ds.ID)
					requireNoScalarSeries(t, series, "datastore_disk_io_read", ds.ID)
					requireNoScalarSeries(t, series, "datastore_disk_iops_reads", ds.ID)
				}

				collecttest.AssertChartCoverage(t, collr, collecttest.ChartCoverageExpectation{})
			},
			checkWithPerf: func(t *testing.T, collr *Collector, series map[string]metrix.SampleValue, count simulator.Model) {
				for _, ds := range collr.resources.Datastores {
					requireScalarSeries(t, series, "datastore_disk_io_read", ds.ID)
				}

				collecttest.AssertChartCoverage(t, collr, collecttest.ChartCoverageExpectation{})
			},
		},
		"clusters": {
			setNoPerfScraper: func(c *Collector) { c.scraper = mockScraperNoClusterPerf{c.scraper} },
			restoreScraper:   func(c *Collector) { c.scraper = mockScraper{c.scraper.(mockScraperNoClusterPerf).scraper} },
			checkNoPerf: func(t *testing.T, collr *Collector, series map[string]metrix.SampleValue, count simulator.Model) {
				for _, cl := range collr.resources.Clusters {
					requireScalarSeries(t, series, "cluster_hosts_total", cl.ID)
					requireScalarSeries(t, series, "cluster_cpu_capacity_total", cl.ID)
					requireScalarSeries(t, series, "cluster_overall_status_green", cl.ID)
					requireNoScalarSeries(t, series, "cluster_cpu_utilization_used", cl.ID)
					requireNoScalarSeries(t, series, "cluster_services_fairness_cpu", cl.ID)
				}

				collecttest.AssertChartCoverage(t, collr, collecttest.ChartCoverageExpectation{})
			},
			checkWithPerf: func(t *testing.T, collr *Collector, series map[string]metrix.SampleValue, count simulator.Model) {
				for _, cl := range collr.resources.Clusters {
					requireScalarSeries(t, series, "cluster_cpu_utilization_used", cl.ID)
				}

				collecttest.AssertChartCoverage(t, collr, collecttest.ChartCoverageExpectation{})
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr, model, teardown := prepareVSphereSim(t)
			defer teardown()

			require.NoError(t, collr.Init(context.Background()))
			tc.setNoPerfScraper(collr)

			mx := collectScalarSeriesForTest(t, collr)
			require.NotNil(t, mx)
			count := model.Count()
			tc.checkNoPerf(t, collr, mx, count)

			tc.restoreScraper(collr)
			mx = collectScalarSeriesForTest(t, collr)
			require.NotNil(t, mx)
			tc.checkWithPerf(t, collr, mx, count)
		})
	}
}
