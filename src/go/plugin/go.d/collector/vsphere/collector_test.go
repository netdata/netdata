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
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/discover"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/match"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

var (
	dataConfigJSON, _       = os.ReadFile("testdata/config.json")
	dataConfigYAML, _       = os.ReadFile("testdata/config.yaml")
	dataV1CompatManifest, _ = os.ReadFile(v1CompatManifestPath)
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":       dataConfigJSON,
		"dataConfigYAML":       dataConfigYAML,
		"dataV1CompatManifest": dataV1CompatManifest,
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
	require.NotEmpty(t, collr.charted)
	require.Greater(t, len(*collr.Charts()), len(inventoryChartsTmpl))

	var keepHostName, keepHostID string
	for _, host := range collr.resources.Hosts {
		keepHostName = host.Name
		keepHostID = host.ID
		break
	}
	require.NotEmpty(t, keepHostName)

	collr.HostsInclude = match.HostIncludes{"/*/*/" + keepHostName}
	require.NoError(t, collr.Init(context.Background()))
	require.Len(t, collr.resources.Hosts, 1)
	require.NotNil(t, collr.resources.Hosts.Get(keepHostID))
	require.Empty(t, collr.charted)
	require.Len(t, *collr.Charts(), len(inventoryChartsTmpl))

	collr.scraper = mockScraper{collr.scraper}
	secondRun := collectScalarSeriesForTest(t, collr)
	require.NotEmpty(t, secondRun)

	numClusters := len(collr.resources.Clusters)
	numResourcePools := len(collr.resources.ResourcePools)
	require.Len(t, collr.charted, len(collr.resources.Hosts)+len(collr.resources.VMs)+len(collr.resources.Datastores)+numClusters+numResourcePools)
	require.Len(t, *collr.Charts(),
		len(inventoryChartsTmpl)+
			len(collr.resources.Hosts)*len(hostChartsTmpl)+
			len(collr.resources.VMs)*len(vmChartsTmpl)+
			len(collr.resources.Datastores)*(len(datastorePropertyChartsTmpl)+len(datastorePerfChartsTmpl))+
			numClusters*(len(clusterPropertyChartsTmpl)+len(clusterPerfChartsTmpl))+
			numResourcePools*len(resourcePoolChartsTmpl))
	for _, chart := range *collr.Charts() {
		if strings.HasPrefix(chart.ID, "host-") {
			require.True(t, strings.HasPrefix(chart.ID, keepHostID+"_"), chart.ID)
		}
	}
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

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
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
	collr, model, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, collr.Init(context.Background()))

	collr.scraper = mockScraper{collr.scraper}

	expected := map[string]int64{
		"inventory_datacenters":                               1,
		"inventory_folders":                                   4,
		"inventory_clusters":                                  1,
		"inventory_hosts":                                     4,
		"inventory_vms":                                       4,
		"inventory_datastores":                                1,
		"inventory_resource_pools":                            1,
		"host-21_cpu.usage.average":                           100,
		"host-21_disk.maxTotalLatency.latest":                 100,
		"host-21_disk.read.average":                           100,
		"host-21_disk.write.average":                          100,
		"host-21_mem.active.average":                          100,
		"host-21_mem.consumed.average":                        100,
		"host-21_mem.granted.average":                         100,
		"host-21_mem.shared.average":                          100,
		"host-21_mem.sharedcommon.average":                    100,
		"host-21_mem.swapinRate.average":                      100,
		"host-21_mem.swapoutRate.average":                     100,
		"host-21_mem.usage.average":                           100,
		"host-21_net.bytesRx.average":                         100,
		"host-21_net.bytesTx.average":                         100,
		"host-21_net.droppedRx.summation":                     100,
		"host-21_net.droppedTx.summation":                     100,
		"host-21_net.errorsRx.summation":                      100,
		"host-21_net.errorsTx.summation":                      100,
		"host-21_net.packetsRx.summation":                     100,
		"host-21_net.packetsTx.summation":                     100,
		"host-21_overall.status.gray":                         1,
		"host-21_overall.status.green":                        0,
		"host-21_overall.status.red":                          0,
		"host-21_overall.status.yellow":                       0,
		"host-21_power_state.poweredOff":                      0,
		"host-21_power_state.poweredOn":                       1,
		"host-21_power_state.standBy":                         0,
		"host-21_power_state.unknown":                         0,
		"host-21_sys.uptime.latest":                           100,
		"host-37_cpu.usage.average":                           100,
		"host-37_disk.maxTotalLatency.latest":                 100,
		"host-37_disk.read.average":                           100,
		"host-37_disk.write.average":                          100,
		"host-37_mem.active.average":                          100,
		"host-37_mem.consumed.average":                        100,
		"host-37_mem.granted.average":                         100,
		"host-37_mem.shared.average":                          100,
		"host-37_mem.sharedcommon.average":                    100,
		"host-37_mem.swapinRate.average":                      100,
		"host-37_mem.swapoutRate.average":                     100,
		"host-37_mem.usage.average":                           100,
		"host-37_net.bytesRx.average":                         100,
		"host-37_net.bytesTx.average":                         100,
		"host-37_net.droppedRx.summation":                     100,
		"host-37_net.droppedTx.summation":                     100,
		"host-37_net.errorsRx.summation":                      100,
		"host-37_net.errorsTx.summation":                      100,
		"host-37_net.packetsRx.summation":                     100,
		"host-37_net.packetsTx.summation":                     100,
		"host-37_overall.status.gray":                         1,
		"host-37_overall.status.green":                        0,
		"host-37_overall.status.red":                          0,
		"host-37_overall.status.yellow":                       0,
		"host-37_power_state.poweredOff":                      0,
		"host-37_power_state.poweredOn":                       1,
		"host-37_power_state.standBy":                         0,
		"host-37_power_state.unknown":                         0,
		"host-37_sys.uptime.latest":                           100,
		"host-47_cpu.usage.average":                           100,
		"host-47_disk.maxTotalLatency.latest":                 100,
		"host-47_disk.read.average":                           100,
		"host-47_disk.write.average":                          100,
		"host-47_mem.active.average":                          100,
		"host-47_mem.consumed.average":                        100,
		"host-47_mem.granted.average":                         100,
		"host-47_mem.shared.average":                          100,
		"host-47_mem.sharedcommon.average":                    100,
		"host-47_mem.swapinRate.average":                      100,
		"host-47_mem.swapoutRate.average":                     100,
		"host-47_mem.usage.average":                           100,
		"host-47_net.bytesRx.average":                         100,
		"host-47_net.bytesTx.average":                         100,
		"host-47_net.droppedRx.summation":                     100,
		"host-47_net.droppedTx.summation":                     100,
		"host-47_net.errorsRx.summation":                      100,
		"host-47_net.errorsTx.summation":                      100,
		"host-47_net.packetsRx.summation":                     100,
		"host-47_net.packetsTx.summation":                     100,
		"host-47_overall.status.gray":                         1,
		"host-47_overall.status.green":                        0,
		"host-47_overall.status.red":                          0,
		"host-47_overall.status.yellow":                       0,
		"host-47_power_state.poweredOff":                      0,
		"host-47_power_state.poweredOn":                       1,
		"host-47_power_state.standBy":                         0,
		"host-47_power_state.unknown":                         0,
		"host-47_sys.uptime.latest":                           100,
		"host-57_cpu.usage.average":                           100,
		"host-57_disk.maxTotalLatency.latest":                 100,
		"host-57_disk.read.average":                           100,
		"host-57_disk.write.average":                          100,
		"host-57_mem.active.average":                          100,
		"host-57_mem.consumed.average":                        100,
		"host-57_mem.granted.average":                         100,
		"host-57_mem.shared.average":                          100,
		"host-57_mem.sharedcommon.average":                    100,
		"host-57_mem.swapinRate.average":                      100,
		"host-57_mem.swapoutRate.average":                     100,
		"host-57_mem.usage.average":                           100,
		"host-57_net.bytesRx.average":                         100,
		"host-57_net.bytesTx.average":                         100,
		"host-57_net.droppedRx.summation":                     100,
		"host-57_net.droppedTx.summation":                     100,
		"host-57_net.errorsRx.summation":                      100,
		"host-57_net.errorsTx.summation":                      100,
		"host-57_net.packetsRx.summation":                     100,
		"host-57_net.packetsTx.summation":                     100,
		"host-57_overall.status.gray":                         1,
		"host-57_overall.status.green":                        0,
		"host-57_overall.status.red":                          0,
		"host-57_overall.status.yellow":                       0,
		"host-57_power_state.poweredOff":                      0,
		"host-57_power_state.poweredOn":                       1,
		"host-57_power_state.standBy":                         0,
		"host-57_power_state.unknown":                         0,
		"host-57_sys.uptime.latest":                           100,
		"vm-62_cpu.usage.average":                             200,
		"vm-62_disk.maxTotalLatency.latest":                   200,
		"vm-62_disk.read.average":                             200,
		"vm-62_disk.write.average":                            200,
		"vm-62_mem.active.average":                            200,
		"vm-62_mem.consumed.average":                          200,
		"vm-62_mem.granted.average":                           200,
		"vm-62_mem.shared.average":                            200,
		"vm-62_mem.swapinRate.average":                        200,
		"vm-62_mem.swapoutRate.average":                       200,
		"vm-62_mem.swapped.average":                           200,
		"vm-62_mem.usage.average":                             200,
		"vm-62_net.bytesRx.average":                           200,
		"vm-62_net.bytesTx.average":                           200,
		"vm-62_net.droppedRx.summation":                       200,
		"vm-62_net.droppedTx.summation":                       200,
		"vm-62_net.packetsRx.summation":                       200,
		"vm-62_net.packetsTx.summation":                       200,
		"vm-62_overall.status.gray":                           0,
		"vm-62_overall.status.green":                          1,
		"vm-62_overall.status.red":                            0,
		"vm-62_overall.status.yellow":                         0,
		"vm-62_power_state.poweredOff":                        0,
		"vm-62_power_state.poweredOn":                         1,
		"vm-62_power_state.suspended":                         0,
		"vm-62_sys.uptime.latest":                             200,
		"vm-62_snapshot_count":                                0,
		"vm-62_snapshot_max_age":                              0,
		"vm-62_snapshot_max_chain_depth":                      0,
		"vm-65_cpu.usage.average":                             200,
		"vm-65_disk.maxTotalLatency.latest":                   200,
		"vm-65_disk.read.average":                             200,
		"vm-65_disk.write.average":                            200,
		"vm-65_mem.active.average":                            200,
		"vm-65_mem.consumed.average":                          200,
		"vm-65_mem.granted.average":                           200,
		"vm-65_mem.shared.average":                            200,
		"vm-65_mem.swapinRate.average":                        200,
		"vm-65_mem.swapoutRate.average":                       200,
		"vm-65_mem.swapped.average":                           200,
		"vm-65_mem.usage.average":                             200,
		"vm-65_net.bytesRx.average":                           200,
		"vm-65_net.bytesTx.average":                           200,
		"vm-65_net.droppedRx.summation":                       200,
		"vm-65_net.droppedTx.summation":                       200,
		"vm-65_net.packetsRx.summation":                       200,
		"vm-65_net.packetsTx.summation":                       200,
		"vm-65_overall.status.gray":                           0,
		"vm-65_overall.status.green":                          1,
		"vm-65_overall.status.red":                            0,
		"vm-65_overall.status.yellow":                         0,
		"vm-65_power_state.poweredOff":                        0,
		"vm-65_power_state.poweredOn":                         1,
		"vm-65_power_state.suspended":                         0,
		"vm-65_sys.uptime.latest":                             200,
		"vm-65_snapshot_count":                                0,
		"vm-65_snapshot_max_age":                              0,
		"vm-65_snapshot_max_chain_depth":                      0,
		"vm-68_cpu.usage.average":                             200,
		"vm-68_disk.maxTotalLatency.latest":                   200,
		"vm-68_disk.read.average":                             200,
		"vm-68_disk.write.average":                            200,
		"vm-68_mem.active.average":                            200,
		"vm-68_mem.consumed.average":                          200,
		"vm-68_mem.granted.average":                           200,
		"vm-68_mem.shared.average":                            200,
		"vm-68_mem.swapinRate.average":                        200,
		"vm-68_mem.swapoutRate.average":                       200,
		"vm-68_mem.swapped.average":                           200,
		"vm-68_mem.usage.average":                             200,
		"vm-68_net.bytesRx.average":                           200,
		"vm-68_net.bytesTx.average":                           200,
		"vm-68_net.droppedRx.summation":                       200,
		"vm-68_net.droppedTx.summation":                       200,
		"vm-68_net.packetsRx.summation":                       200,
		"vm-68_net.packetsTx.summation":                       200,
		"vm-68_overall.status.gray":                           0,
		"vm-68_overall.status.green":                          1,
		"vm-68_overall.status.red":                            0,
		"vm-68_overall.status.yellow":                         0,
		"vm-68_power_state.poweredOff":                        0,
		"vm-68_power_state.poweredOn":                         1,
		"vm-68_power_state.suspended":                         0,
		"vm-68_sys.uptime.latest":                             200,
		"vm-68_snapshot_count":                                0,
		"vm-68_snapshot_max_age":                              0,
		"vm-68_snapshot_max_chain_depth":                      0,
		"vm-71_cpu.usage.average":                             200,
		"vm-71_disk.maxTotalLatency.latest":                   200,
		"vm-71_disk.read.average":                             200,
		"vm-71_disk.write.average":                            200,
		"vm-71_mem.active.average":                            200,
		"vm-71_mem.consumed.average":                          200,
		"vm-71_mem.granted.average":                           200,
		"vm-71_mem.shared.average":                            200,
		"vm-71_mem.swapinRate.average":                        200,
		"vm-71_mem.swapoutRate.average":                       200,
		"vm-71_mem.swapped.average":                           200,
		"vm-71_mem.usage.average":                             200,
		"vm-71_net.bytesRx.average":                           200,
		"vm-71_net.bytesTx.average":                           200,
		"vm-71_net.droppedRx.summation":                       200,
		"vm-71_net.droppedTx.summation":                       200,
		"vm-71_net.packetsRx.summation":                       200,
		"vm-71_net.packetsTx.summation":                       200,
		"vm-71_overall.status.gray":                           0,
		"vm-71_overall.status.green":                          1,
		"vm-71_overall.status.red":                            0,
		"vm-71_overall.status.yellow":                         0,
		"vm-71_power_state.poweredOff":                        0,
		"vm-71_power_state.poweredOn":                         1,
		"vm-71_power_state.suspended":                         0,
		"vm-71_sys.uptime.latest":                             200,
		"vm-71_snapshot_count":                                0,
		"vm-71_snapshot_max_age":                              0,
		"vm-71_snapshot_max_chain_depth":                      0,
		"datastore-59_capacity":                               4398046511104,
		"datastore-59_free_space":                             4355096838144,
		"datastore-59_used_space":                             42949672960,
		"datastore-59_used_space_pct":                         97,
		"datastore-59_uncommitted":                            0,
		"datastore-59_overall.status.green":                   1,
		"datastore-59_overall.status.gray":                    0,
		"datastore-59_overall.status.red":                     0,
		"datastore-59_overall.status.yellow":                  0,
		"datastore-59_accessible_status.accessible":           1,
		"datastore-59_accessible_status.inaccessible":         0,
		"datastore-59_maintenance.status.normal":              1,
		"datastore-59_maintenance.status.enteringMaintenance": 0,
		"datastore-59_maintenance.status.inMaintenance":       0,
		"datastore-59_maintenance.status.unknown":             0,
		"datastore-59_multiple_host_access.enabled":           0,
		"datastore-59_multiple_host_access.disabled":          0,
		"datastore-59_multiple_host_access.unknown":           1,
		"datastore-59_datastore.numberReadAveraged.average":   300,
		"datastore-59_datastore.numberWriteAveraged.average":  300,
		"datastore-59_datastore.read.average":                 300,
		"datastore-59_datastore.write.average":                300,
		"datastore-59_datastore.totalReadLatency.average":     300,
		"datastore-59_datastore.totalWriteLatency.average":    300,
		// Cluster property metrics (domain-c28)
		"domain-c28_num_hosts":                  3,
		"domain-c28_num_effective_hosts":        3,
		"domain-c28_total_cpu":                  6882,
		"domain-c28_effective_cpu":              6882,
		"domain-c28_total_memory":               12883292160,
		"domain-c28_effective_memory":           13509110959964160,
		"domain-c28_num_cpu_cores":              6,
		"domain-c28_num_cpu_threads":            6,
		"domain-c28_num_vmotions":               0,
		"domain-c28_drs_score":                  0,
		"domain-c28_current_balance":            0,
		"domain-c28_target_balance":             0,
		"domain-c28_drs_enabled":                1,
		"domain-c28_ha_enabled":                 0,
		"domain-c28_ha_adm_ctrl_enabled":        0,
		"domain-c28_usage_cpu_demand_mhz":       0,
		"domain-c28_usage_mem_demand_mb":        0,
		"domain-c28_usage_cpu_entitled_mhz":     0,
		"domain-c28_usage_mem_entitled_mb":      0,
		"domain-c28_usage_cpu_reservation_mhz":  0,
		"domain-c28_usage_mem_reservation_mb":   0,
		"domain-c28_usage_total_vm_count":       0,
		"domain-c28_usage_powered_off_vm_count": 0,
		"domain-c28_overall.status.green":       1,
		"domain-c28_overall.status.gray":        0,
		"domain-c28_overall.status.red":         0,
		"domain-c28_overall.status.yellow":      0,
		// Cluster perf metrics (domain-c28)
		"domain-c28_clusterServices.cpufairness.latest":   400,
		"domain-c28_clusterServices.effectivecpu.average": 400,
		"domain-c28_clusterServices.effectivemem.average": 400,
		"domain-c28_clusterServices.failover.latest":      400,
		"domain-c28_clusterServices.memfairness.latest":   400,
		"domain-c28_cpu.totalmhz.average":                 400,
		"domain-c28_cpu.usage.average":                    400,
		"domain-c28_cpu.usagemhz.average":                 400,
		"domain-c28_mem.active.average":                   400,
		"domain-c28_mem.consumed.average":                 400,
		"domain-c28_mem.granted.average":                  400,
		"domain-c28_mem.overhead.average":                 400,
		"domain-c28_mem.shared.average":                   400,
		"domain-c28_mem.swapused.average":                 400,
		"domain-c28_mem.usage.average":                    400,
		"domain-c28_vmop.numChangeDS.latest":              400,
		"domain-c28_vmop.numChangeHost.latest":            400,
		"domain-c28_vmop.numChangeHostDS.latest":          400,
		"domain-c28_vmop.numClone.latest":                 400,
		"domain-c28_vmop.numCreate.latest":                400,
		"domain-c28_vmop.numDeploy.latest":                400,
		"domain-c28_vmop.numDestroy.latest":               400,
		"domain-c28_vmop.numPoweroff.latest":              400,
		"domain-c28_vmop.numPoweron.latest":               400,
		"domain-c28_vmop.numRebootGuest.latest":           400,
		"domain-c28_vmop.numReconfigure.latest":           400,
		"domain-c28_vmop.numRegister.latest":              400,
		"domain-c28_vmop.numReset.latest":                 400,
		"domain-c28_vmop.numSVMotion.latest":              400,
		"domain-c28_vmop.numShutdownGuest.latest":         400,
		"domain-c28_vmop.numStandbyGuest.latest":          400,
		"domain-c28_vmop.numSuspend.latest":               400,
		"domain-c28_vmop.numUnregister.latest":            400,
		"domain-c28_vmop.numVMotion.latest":               400,
		"domain-c28_vmop.numXVMotion.latest":              400,
		// Resource pool metrics (resgroup-27)
		"resgroup-27_cpu_usage":                   0,
		"resgroup-27_cpu_demand":                  0,
		"resgroup-27_cpu_entitlement_distributed": 0,
		"resgroup-27_mem_usage_guest":             0,
		"resgroup-27_mem_usage_host":              0,
		"resgroup-27_mem_entitlement_distributed": 0,
		"resgroup-27_mem_private":                 0,
		"resgroup-27_mem_shared":                  0,
		"resgroup-27_mem_swapped":                 0,
		"resgroup-27_mem_ballooned":               0,
		"resgroup-27_mem_overhead":                0,
		"resgroup-27_mem_consumed_overhead":       0,
		"resgroup-27_mem_compressed":              0,
		"resgroup-27_cpu_reservation_used":        0,
		"resgroup-27_cpu_max_usage":               4121,
		"resgroup-27_cpu_unreserved_for_vm":       4121,
		"resgroup-27_mem_reservation_used":        0,
		"resgroup-27_mem_max_usage":               1007681536,
		"resgroup-27_mem_unreserved_for_vm":       1007681536,
		"resgroup-27_cpu_reservation":             4121,
		"resgroup-27_cpu_limit":                   4121,
		"resgroup-27_mem_reservation":             961,
		"resgroup-27_mem_limit":                   961,
		"resgroup-27_overall.status.green":        1,
		"resgroup-27_overall.status.gray":         0,
		"resgroup-27_overall.status.red":          0,
		"resgroup-27_overall.status.yellow":       0,
	}
	for _, id := range []string{"host-21", "host-37", "host-47", "host-57"} {
		addExpectedHostRuntimeStatus(expected, id)
	}
	for _, id := range []string{"vm-62", "vm-65", "vm-68", "vm-71"} {
		addExpectedVMPropertyStatus(expected, id)
	}
	addExpectedClusterConfigStatus(expected, "domain-c28")

	mx := collectV1MapForTest(t, collr)

	require.Equal(t, expected, mx)

	count := model.Count()
	assert.Len(t, collr.discoveredHosts, count.Host)
	assert.Len(t, collr.discoveredVMs, count.Machine)
	assert.Len(t, collr.discoveredDatastores, count.Datastore)

	numClusters := len(collr.discoveredClusters)
	numResourcePools := len(collr.discoveredResourcePools)
	assert.Len(t, collr.charted, count.Host+count.Machine+count.Datastore+numClusters+numResourcePools)

	assert.Len(t, *collr.Charts(),
		len(inventoryChartsTmpl)+
			count.Host*len(hostChartsTmpl)+
			count.Machine*len(vmChartsTmpl)+
			count.Datastore*(len(datastorePropertyChartsTmpl)+len(datastorePerfChartsTmpl))+
			numClusters*(len(clusterPropertyChartsTmpl)+len(clusterPerfChartsTmpl))+
			numResourcePools*len(resourcePoolChartsTmpl))
	collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
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
	mx := make(map[string]int64)

	collr.collectInventory(mx)

	assert.EqualValues(t, 1, mx["inventory_datacenters"])
	assert.EqualValues(t, 2, mx["inventory_folders"])
	assert.EqualValues(t, 1, mx["inventory_clusters"])
	assert.EqualValues(t, 2, mx["inventory_hosts"])
	assert.EqualValues(t, 3, mx["inventory_vms"])
	assert.EqualValues(t, 1, mx["inventory_datastores"])
	assert.EqualValues(t, 1, mx["inventory_resource_pools"])
}

func TestCollector_Collect_NonPoweredResourcePropertyOnly(t *testing.T) {
	tests := map[string]struct {
		setup   func(*Collector)
		collect func(*Collector, map[string]int64) error
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
				"host-1_overall.status.gray":    1,
				"host-1_power_state.poweredOff": 1,
				"host-1_power_state.poweredOn":  0,
			},
			missing: []string{"host-1_cpu.usage.average"},
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
				"vm-1_overall.status.yellow":    1,
				"vm-1_power_state.suspended":    1,
				"vm-1_power_state.poweredOn":    0,
				"vm-1_snapshot_count":           2,
				"vm-1_snapshot_max_chain_depth": 1,
			},
			missing: []string{"vm-1_cpu.usage.average"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			tc.setup(collr)
			mx := make(map[string]int64)

			require.NoError(t, tc.collect(collr, mx))

			for key, want := range tc.want {
				assert.EqualValues(t, want, mx[key])
			}
			for _, key := range tc.missing {
				assert.NotContains(t, mx, key)
			}
		})
	}
}

func TestCollector_Collect_NoPerfDataKeepsPropertyMetrics(t *testing.T) {
	tests := map[string]struct {
		setup func(*Collector)
		check func(*testing.T, *Collector, map[string]int64)
	}{
		"hosts": {
			setup: func(c *Collector) { c.scraper = mockScraperNoHostPerf{mockScraper{c.scraper}} },
			check: func(t *testing.T, collr *Collector, mx map[string]int64) {
				for _, host := range collr.resources.Hosts {
					assert.Contains(t, mx, host.ID+"_overall.status.green")
					assert.Contains(t, mx, host.ID+"_power_state.poweredOn")
					assert.NotContains(t, mx, host.ID+"_cpu.usage.average")
				}
				for _, vm := range collr.resources.VMs {
					assert.Contains(t, mx, vm.ID+"_cpu.usage.average")
				}
			},
		},
		"VMs": {
			setup: func(c *Collector) { c.scraper = mockScraperNoVMPerf{mockScraper{c.scraper}} },
			check: func(t *testing.T, collr *Collector, mx map[string]int64) {
				for _, host := range collr.resources.Hosts {
					assert.Contains(t, mx, host.ID+"_cpu.usage.average")
				}
				for _, vm := range collr.resources.VMs {
					assert.Contains(t, mx, vm.ID+"_overall.status.green")
					assert.Contains(t, mx, vm.ID+"_power_state.poweredOn")
					assert.NotContains(t, mx, vm.ID+"_cpu.usage.average")
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

			mx := collectV1MapForTest(t, collr)
			require.NotNil(t, mx)

			tc.check(t, collr, mx)
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
			collect: func(c *Collector) error { return c.collectHosts(make(map[string]int64)) },
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
			collect: func(c *Collector) error { return c.collectVMs(make(map[string]int64)) },
			wantLog: "collect VM performance metrics",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			collr := New()
			collr.Logger = logger.NewWithWriter(&buf)
			tc.setup(collr)

			require.NoError(t, tc.collect(collr))
			require.NoError(t, tc.collect(collr))

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
	mx := make(map[string]int64)

	writeHostPropertyMetrics(mx, host)

	assert.EqualValues(t, 1, mx["host-1_connection_state.notResponding"])
	assert.EqualValues(t, 0, mx["host-1_connection_state.connected"])
	assert.EqualValues(t, 1, mx["host-1_maintenance_status.inMaintenance"])
	assert.EqualValues(t, 0, mx["host-1_maintenance_status.normal"])
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
	mx := make(map[string]int64)

	writeVMPropertyMetrics(mx, vm)

	assert.EqualValues(t, 1, mx["vm-1_connection_state.inaccessible"])
	assert.EqualValues(t, 0, mx["vm-1_connection_state.connected"])
	assert.EqualValues(t, 1, mx["vm-1_tools_running_status.running"])
	assert.EqualValues(t, 0, mx["vm-1_tools_running_status.unknown"])
	assert.EqualValues(t, 1, mx["vm-1_tools_version_status.tooOld"])
	assert.EqualValues(t, 0, mx["vm-1_tools_version_status.unknown"])
	assert.EqualValues(t, 1, mx["vm-1_consolidation_needed.needed"])
	assert.EqualValues(t, 0, mx["vm-1_consolidation_needed.notNeeded"])
	assert.EqualValues(t, 4, mx["vm-1_config_cpu"])
	assert.EqualValues(t, 8192, mx["vm-1_config_memory"])
	assert.EqualValues(t, 2, mx["vm-1_config_devices.disks"])
	assert.EqualValues(t, 3, mx["vm-1_config_devices.nics"])
	assert.EqualValues(t, 100, mx["vm-1_storage.committed"])
	assert.EqualValues(t, 200, mx["vm-1_storage.uncommitted"])
	assert.EqualValues(t, 300, mx["vm-1_storage.unshared"])
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
	mx := make(map[string]int64)

	writeClusterPropertyMetrics(mx, cluster)

	assert.EqualValues(t, 1, mx["domain-c1_drs_enabled"])
	assert.EqualValues(t, 1, mx["domain-c1_drs_mode.fullyAutomated"])
	assert.EqualValues(t, 0, mx["domain-c1_drs_mode.unknown"])
	assert.EqualValues(t, 3, mx["domain-c1_drs_vmotion_rate"])
	assert.EqualValues(t, 1, mx["domain-c1_ha_enabled"])
	assert.EqualValues(t, 1, mx["domain-c1_ha_adm_ctrl_enabled"])
	assert.EqualValues(t, 1, mx["domain-c1_ha_host_monitoring.enabled"])
	assert.EqualValues(t, 0, mx["domain-c1_ha_host_monitoring.unknown"])
	assert.EqualValues(t, 1, mx["domain-c1_ha_vm_monitoring.vmAndAppMonitoring"])
	assert.EqualValues(t, 0, mx["domain-c1_ha_vm_monitoring.unknown"])
	assert.EqualValues(t, 1, mx["domain-c1_ha_vm_component_protection.disabled"])
	assert.EqualValues(t, 0, mx["domain-c1_ha_vm_component_protection.unknown"])
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

func collectV1MapForTest(t *testing.T, collr *Collector) map[string]int64 {
	t.Helper()

	cycle := mustCycleController(t, collr.MetricStore())
	cycle.BeginCycle()

	mx, err := collr.collect()
	if err != nil {
		cycle.AbortCycle()
		require.NoError(t, err)
	}

	collr.writeMetrics(mx)
	require.NoError(t, cycle.CommitCycleSuccess())
	if len(mx) == 0 {
		return nil
	}
	return mx
}

func addExpectedHostRuntimeStatus(mx map[string]int64, id string) {
	mx[fmt.Sprintf("%s_connection_state.connected", id)] = 1
	mx[fmt.Sprintf("%s_connection_state.disconnected", id)] = 0
	mx[fmt.Sprintf("%s_connection_state.notResponding", id)] = 0
	mx[fmt.Sprintf("%s_maintenance_status.inMaintenance", id)] = 0
	mx[fmt.Sprintf("%s_maintenance_status.normal", id)] = 1
}

func addExpectedVMPropertyStatus(mx map[string]int64, id string) {
	mx[fmt.Sprintf("%s_connection_state.connected", id)] = 1
	mx[fmt.Sprintf("%s_connection_state.disconnected", id)] = 0
	mx[fmt.Sprintf("%s_connection_state.inaccessible", id)] = 0
	mx[fmt.Sprintf("%s_connection_state.invalid", id)] = 0
	mx[fmt.Sprintf("%s_connection_state.orphaned", id)] = 0
	mx[fmt.Sprintf("%s_tools_running_status.executingScripts", id)] = 0
	mx[fmt.Sprintf("%s_tools_running_status.notRunning", id)] = 0
	mx[fmt.Sprintf("%s_tools_running_status.running", id)] = 0
	mx[fmt.Sprintf("%s_tools_running_status.unknown", id)] = 1
	mx[fmt.Sprintf("%s_tools_version_status.blacklisted", id)] = 0
	mx[fmt.Sprintf("%s_tools_version_status.current", id)] = 0
	mx[fmt.Sprintf("%s_tools_version_status.needUpgrade", id)] = 0
	mx[fmt.Sprintf("%s_tools_version_status.notInstalled", id)] = 0
	mx[fmt.Sprintf("%s_tools_version_status.supportedNew", id)] = 0
	mx[fmt.Sprintf("%s_tools_version_status.supportedOld", id)] = 0
	mx[fmt.Sprintf("%s_tools_version_status.tooNew", id)] = 0
	mx[fmt.Sprintf("%s_tools_version_status.tooOld", id)] = 0
	mx[fmt.Sprintf("%s_tools_version_status.unknown", id)] = 1
	mx[fmt.Sprintf("%s_tools_version_status.unmanaged", id)] = 0
	mx[fmt.Sprintf("%s_consolidation_needed.needed", id)] = 0
	mx[fmt.Sprintf("%s_consolidation_needed.notNeeded", id)] = 1
	mx[fmt.Sprintf("%s_config_cpu", id)] = 1
	mx[fmt.Sprintf("%s_config_memory", id)] = 32
	mx[fmt.Sprintf("%s_config_devices.disks", id)] = 1
	mx[fmt.Sprintf("%s_config_devices.nics", id)] = 1
	mx[fmt.Sprintf("%s_storage.committed", id)] = 234
	mx[fmt.Sprintf("%s_storage.uncommitted", id)] = 10737418240
	mx[fmt.Sprintf("%s_storage.unshared", id)] = 202
}

func addExpectedClusterConfigStatus(mx map[string]int64, id string) {
	mx[fmt.Sprintf("%s_drs_mode.fullyAutomated", id)] = 0
	mx[fmt.Sprintf("%s_drs_mode.manual", id)] = 0
	mx[fmt.Sprintf("%s_drs_mode.partiallyAutomated", id)] = 0
	mx[fmt.Sprintf("%s_drs_mode.unknown", id)] = 1
	mx[fmt.Sprintf("%s_drs_vmotion_rate", id)] = 0
	mx[fmt.Sprintf("%s_ha_host_monitoring.disabled", id)] = 0
	mx[fmt.Sprintf("%s_ha_host_monitoring.enabled", id)] = 0
	mx[fmt.Sprintf("%s_ha_host_monitoring.unknown", id)] = 1
	mx[fmt.Sprintf("%s_ha_vm_component_protection.disabled", id)] = 0
	mx[fmt.Sprintf("%s_ha_vm_component_protection.enabled", id)] = 0
	mx[fmt.Sprintf("%s_ha_vm_component_protection.unknown", id)] = 1
	mx[fmt.Sprintf("%s_ha_vm_monitoring.unknown", id)] = 1
	mx[fmt.Sprintf("%s_ha_vm_monitoring.vmAndAppMonitoring", id)] = 0
	mx[fmt.Sprintf("%s_ha_vm_monitoring.vmMonitoringDisabled", id)] = 0
	mx[fmt.Sprintf("%s_ha_vm_monitoring.vmMonitoringOnly", id)] = 0
}

func mustCycleController(t *testing.T, store metrix.CollectorStore) metrix.CycleController {
	t.Helper()

	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok)
	return managed.CycleController()
}

func TestCollector_Collect_RemoveHostsVMsInRuntime(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	okHostId := "host-57"
	okVmId := "vm-62"
	collr.discoverer.(*discover.Discoverer).HostMatcher = mockHostMatcher{okHostId}
	collr.discoverer.(*discover.Discoverer).VMMatcher = mockVMMatcher{okVmId}

	require.NoError(t, collr.discoverOnce())

	numOfRuns := 5
	for range numOfRuns {
		collectScalarSeriesForTest(t, collr)
	}

	host := collr.resources.Hosts.Get(okHostId)
	for k, v := range collr.discoveredHosts {
		if k == host.ID {
			assert.Equal(t, 0, v)
		} else {
			assert.Equal(t, numOfRuns, v)
		}
	}

	vm := collr.resources.VMs.Get(okVmId)
	for id, fails := range collr.discoveredVMs {
		if id == vm.ID {
			assert.Equal(t, 0, fails)
		} else {
			assert.Equal(t, numOfRuns, fails)
		}

	}

	for i := numOfRuns; i < failedUpdatesLimit; i++ {
		collectScalarSeriesForTest(t, collr)
	}

	assert.Len(t, collr.discoveredHosts, 1)
	assert.Len(t, collr.discoveredVMs, 1)
	assert.Len(t, collr.charted, 2+len(collr.discoveredDatastores)+len(collr.discoveredClusters)+len(collr.discoveredResourcePools))

	for _, c := range *collr.Charts() {
		if c.ID == "inventory_objects" || strings.HasPrefix(c.ID, okHostId) || strings.HasPrefix(c.ID, okVmId) ||
			strings.HasPrefix(c.ID, "datastore-") || strings.HasPrefix(c.ID, "domain-") || strings.HasPrefix(c.ID, "resgroup-") {
			assert.False(t, c.Obsolete)
		} else {
			assert.True(t, c.Obsolete)
		}
	}
}

func TestCollector_Collect_Run(t *testing.T) {
	collr, model, teardown := prepareVSphereSim(t)
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

	count := model.Count()
	assert.Len(t, collr.discoveredHosts, count.Host)
	assert.Len(t, collr.discoveredVMs, count.Machine)
	assert.Len(t, collr.discoveredDatastores, count.Datastore)

	numClusters := len(collr.discoveredClusters)
	numResourcePools := len(collr.discoveredResourcePools)
	assert.Len(t, collr.charted, count.Host+count.Machine+count.Datastore+numClusters+numResourcePools)
	assert.Len(t, *collr.charts,
		len(inventoryChartsTmpl)+
			count.Host*len(hostChartsTmpl)+
			count.Machine*len(vmChartsTmpl)+
			count.Datastore*(len(datastorePropertyChartsTmpl)+len(datastorePerfChartsTmpl))+
			numClusters*(len(clusterPropertyChartsTmpl)+len(clusterPerfChartsTmpl))+
			numResourcePools*len(resourcePoolChartsTmpl))
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

func TestCollector_Collect_DatastoreNoPerfData(t *testing.T) {
	collr, model, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, collr.Init(context.Background()))

	collr.scraper = mockScraperNoDSPerf{collr.scraper}

	mx := collectV1MapForTest(t, collr)
	require.NotNil(t, mx)

	count := model.Count()

	// Property metrics should be present.
	for _, ds := range collr.resources.Datastores {
		assert.Contains(t, mx, ds.ID+"_capacity")
		assert.Contains(t, mx, ds.ID+"_free_space")
		assert.Contains(t, mx, ds.ID+"_used_space")
		assert.Contains(t, mx, ds.ID+"_used_space_pct")
		assert.Contains(t, mx, ds.ID+"_uncommitted")
		assert.Contains(t, mx, ds.ID+"_overall.status.green")
		assert.Contains(t, mx, ds.ID+"_accessible_status.accessible")
		assert.Contains(t, mx, ds.ID+"_maintenance.status.normal")
		assert.Contains(t, mx, ds.ID+"_multiple_host_access.unknown")
	}

	// Perf metrics should NOT be present.
	for _, ds := range collr.resources.Datastores {
		assert.NotContains(t, mx, ds.ID+"_datastore.read.average")
		assert.NotContains(t, mx, ds.ID+"_datastore.numberReadAveraged.average")
	}

	numClusters := len(collr.discoveredClusters)
	numResourcePools := len(collr.discoveredResourcePools)

	// Only property charts created for datastores, no perf charts.
	assert.Len(t, *collr.Charts(),
		len(inventoryChartsTmpl)+
			count.Host*len(hostChartsTmpl)+
			count.Machine*len(vmChartsTmpl)+
			count.Datastore*len(datastorePropertyChartsTmpl)+
			numClusters*(len(clusterPropertyChartsTmpl)+len(clusterPerfChartsTmpl))+
			numResourcePools*len(resourcePoolChartsTmpl))

	// datastorePerfReceived should be empty.
	assert.Empty(t, collr.datastorePerfReceived)
	assert.Empty(t, collr.datastorePerfCharted)

	// Now switch to a scraper that returns perf data — perf charts should appear.
	collr.scraper = mockScraper{collr.scraper.(mockScraperNoDSPerf).scraper}

	mx = collectV1MapForTest(t, collr)
	require.NotNil(t, mx)

	// Perf metrics should now be present.
	for _, ds := range collr.resources.Datastores {
		assert.Contains(t, mx, ds.ID+"_datastore.read.average")
	}

	// Both property and perf charts now for datastores.
	assert.Len(t, *collr.Charts(),
		len(inventoryChartsTmpl)+
			count.Host*len(hostChartsTmpl)+
			count.Machine*len(vmChartsTmpl)+
			count.Datastore*(len(datastorePropertyChartsTmpl)+len(datastorePerfChartsTmpl))+
			numClusters*(len(clusterPropertyChartsTmpl)+len(clusterPerfChartsTmpl))+
			numResourcePools*len(resourcePoolChartsTmpl))

	assert.Len(t, collr.datastorePerfReceived, count.Datastore)
	assert.Len(t, collr.datastorePerfCharted, count.Datastore)
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

func TestCollector_Collect_ClusterNoPerfData(t *testing.T) {
	collr, model, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, collr.Init(context.Background()))

	collr.scraper = mockScraperNoClusterPerf{collr.scraper}

	mx := collectV1MapForTest(t, collr)
	require.NotNil(t, mx)

	count := model.Count()

	// Cluster property metrics should be present.
	for _, cl := range collr.resources.Clusters {
		assert.Contains(t, mx, cl.ID+"_num_hosts")
		assert.Contains(t, mx, cl.ID+"_total_cpu")
		assert.Contains(t, mx, cl.ID+"_overall.status.green")
	}

	// Cluster perf metrics should NOT be present.
	for _, cl := range collr.resources.Clusters {
		assert.NotContains(t, mx, cl.ID+"_cpu.usage.average")
		assert.NotContains(t, mx, cl.ID+"_clusterServices.cpufairness.latest")
	}

	numClusters := len(collr.discoveredClusters)
	numResourcePools := len(collr.discoveredResourcePools)

	// Only property charts created for clusters, no perf charts.
	assert.Len(t, *collr.Charts(),
		len(inventoryChartsTmpl)+
			count.Host*len(hostChartsTmpl)+
			count.Machine*len(vmChartsTmpl)+
			count.Datastore*(len(datastorePropertyChartsTmpl)+len(datastorePerfChartsTmpl))+
			numClusters*len(clusterPropertyChartsTmpl)+
			numResourcePools*len(resourcePoolChartsTmpl))

	// clusterPerfReceived should be empty.
	assert.Empty(t, collr.clusterPerfReceived)
	assert.Empty(t, collr.clusterPerfCharted)

	// Now switch to a scraper that returns perf data — perf charts should appear.
	collr.scraper = mockScraper{collr.scraper.(mockScraperNoClusterPerf).scraper}

	mx = collectV1MapForTest(t, collr)
	require.NotNil(t, mx)

	// Perf metrics should now be present.
	for _, cl := range collr.resources.Clusters {
		assert.Contains(t, mx, cl.ID+"_cpu.usage.average")
	}

	// Both property and perf charts now for clusters.
	assert.Len(t, *collr.Charts(),
		len(inventoryChartsTmpl)+
			count.Host*len(hostChartsTmpl)+
			count.Machine*len(vmChartsTmpl)+
			count.Datastore*(len(datastorePropertyChartsTmpl)+len(datastorePerfChartsTmpl))+
			numClusters*(len(clusterPropertyChartsTmpl)+len(clusterPerfChartsTmpl))+
			numResourcePools*len(resourcePoolChartsTmpl))

	assert.Len(t, collr.clusterPerfReceived, numClusters)
	assert.Len(t, collr.clusterPerfCharted, numClusters)
}

func TestCollector_Collect_ClusterEvictionCleansUpMaps(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, collr.Init(context.Background()))

	collr.scraper = mockScraper{collr.scraper}

	// First collect — creates all charts including cluster perf charts.
	mx := collectV1MapForTest(t, collr)
	require.NotNil(t, mx)

	assert.NotEmpty(t, collr.discoveredClusters)
	assert.NotEmpty(t, collr.clusterPerfReceived)
	assert.NotEmpty(t, collr.clusterPerfCharted)

	// Simulate eviction: disable property collector and perf scraper so the counter isn't reset,
	// then set the failure counter to the eviction threshold.
	collr.clusterPropertyCollector = nil
	collr.scraper = mockScraperNoClusterPerf{collr.scraper.(mockScraper).scraper}
	for id := range collr.discoveredClusters {
		collr.discoveredClusters[id] = failedUpdatesLimit
	}

	// Next collect increments counter past the limit, triggering eviction in updateCharts.
	collectScalarSeriesForTest(t, collr)

	assert.Empty(t, collr.discoveredClusters)
	assert.Empty(t, collr.clusterPerfReceived)
	assert.Empty(t, collr.clusterPerfCharted)

	// Cluster charts should be marked obsolete.
	for _, c := range *collr.Charts() {
		if strings.HasPrefix(c.ID, "domain-") {
			assert.True(t, c.Obsolete, "chart %s should be obsolete", c.ID)
		}
	}
}

func TestCollector_Collect_ResourcePoolEvictionCleansUpMaps(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, collr.Init(context.Background()))

	collr.scraper = mockScraper{collr.scraper}

	// First collect — creates resource pool charts.
	mx := collectScalarSeriesForTest(t, collr)
	require.NotNil(t, mx)

	assert.NotEmpty(t, collr.discoveredResourcePools)

	// Simulate eviction: disable property collector so the counter isn't reset.
	collr.rpPropertyCollector = nil
	for id := range collr.discoveredResourcePools {
		collr.discoveredResourcePools[id] = failedUpdatesLimit
	}

	// Next collect increments counter past the limit, triggering eviction.
	collectScalarSeriesForTest(t, collr)

	assert.Empty(t, collr.discoveredResourcePools)

	// Resource pool charts should be marked obsolete.
	for _, c := range *collr.Charts() {
		if strings.HasPrefix(c.ID, "resgroup-") {
			assert.True(t, c.Obsolete, "chart %s should be obsolete", c.ID)
		}
	}
}

func TestCollector_Collect_DatastoreEvictionCleansUpMaps(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, collr.Init(context.Background()))

	collr.scraper = mockScraper{collr.scraper}

	// First collect — creates all charts including datastore perf charts.
	mx := collectScalarSeriesForTest(t, collr)
	require.NotNil(t, mx)

	assert.NotEmpty(t, collr.discoveredDatastores)
	assert.NotEmpty(t, collr.datastorePerfReceived)
	assert.NotEmpty(t, collr.datastorePerfCharted)

	// Simulate eviction: disable property collector and perf scraper so the counter isn't reset,
	// then set the failure counter to the eviction threshold.
	collr.dsPropertyCollector = nil
	collr.scraper = mockScraperNoDSPerf{collr.scraper.(mockScraper).scraper}
	for id := range collr.discoveredDatastores {
		collr.discoveredDatastores[id] = failedUpdatesLimit
	}

	// Next collect increments counter past the limit, triggering eviction in updateCharts.
	collectScalarSeriesForTest(t, collr)

	assert.Empty(t, collr.discoveredDatastores)
	assert.Empty(t, collr.datastorePerfReceived)
	assert.Empty(t, collr.datastorePerfCharted)

	// Datastore charts should be marked obsolete.
	for _, c := range *collr.Charts() {
		if strings.HasPrefix(c.ID, "datastore-") {
			assert.True(t, c.Obsolete, "chart %s should be obsolete", c.ID)
		}
	}
}
