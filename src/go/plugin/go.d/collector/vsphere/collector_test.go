// SPDX-License-Identifier: GPL-3.0-or-later
package vsphere

import (
	"context"
	"crypto/tls"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware/govmomi/performance"
	"github.com/vmware/govmomi/simulator"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/discover"
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
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()

	assert.NoError(t, collr.Init(context.Background()))
	assert.NotNil(t, collr.discoverer)
	assert.NotNil(t, collr.scraper)
	assert.NotNil(t, collr.resources)
	assert.NotNil(t, collr.discoveryTask)
	assert.True(t, collr.discoveryTask.isRunning())
}

func TestCollector_Init_ReturnsFalseIfURLNotSet(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()
	collr.URL = ""

	assert.Error(t, collr.Init(context.Background()))
}

func TestCollector_Init_ReturnsFalseIfUsernameNotSet(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()
	collr.Username = ""

	assert.Error(t, collr.Init(context.Background()))
}

func TestCollector_Init_ReturnsFalseIfPasswordNotSet(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()
	collr.Password = ""

	assert.Error(t, collr.Init(context.Background()))
}

func TestCollector_Init_ReturnsFalseIfClientWrongTLSCA(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()
	collr.ClientConfig.TLSConfig.TLSCA = "testdata/tls"

	assert.Error(t, collr.Init(context.Background()))
}

func TestCollector_Init_ReturnsFalseIfConnectionRefused(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()
	collr.URL = "http://127.0.0.1:32001"

	assert.Error(t, collr.Init(context.Background()))
}

func TestCollector_Init_ReturnsFalseIfInvalidHostVMIncludeFormat(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()

	collr.HostsInclude = match.HostIncludes{"invalid"}
	assert.Error(t, collr.Init(context.Background()))

	collr.HostsInclude = collr.HostsInclude[:0]

	collr.VMsInclude = match.VMIncludes{"invalid"}
	assert.Error(t, collr.Init(context.Background()))

	collr.VMsInclude = collr.VMsInclude[:0]

	collr.DatastoresInclude = match.DatastoreIncludes{"invalid"}
	assert.Error(t, collr.Init(context.Background()))

	collr.DatastoresInclude = collr.DatastoresInclude[:0]

	collr.ClustersInclude = match.ClusterIncludes{"invalid"}
	assert.Error(t, collr.Init(context.Background()))
}

func TestCollector_Check(t *testing.T) {
	assert.NoError(t, New().Check(context.Background()))
}

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Cleanup(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, collr.Init(context.Background()))

	collr.Cleanup(context.Background())
	time.Sleep(time.Second)
	assert.True(t, collr.discoveryTask.isStopped())
	assert.False(t, collr.discoveryTask.isRunning())
}

func TestCollector_Cleanup_NotPanicsIfNotInitialized(t *testing.T) {
	assert.NotPanics(t, func() { New().Cleanup(context.Background()) })
}

func TestCollector_Collect(t *testing.T) {
	collr, model, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, collr.Init(context.Background()))

	collr.scraper = mockScraper{collr.scraper}

	expected := map[string]int64{
		"host-21_cpu.usage.average":                          100,
		"host-21_disk.maxTotalLatency.latest":                100,
		"host-21_disk.read.average":                          100,
		"host-21_disk.write.average":                         100,
		"host-21_mem.active.average":                         100,
		"host-21_mem.consumed.average":                       100,
		"host-21_mem.granted.average":                        100,
		"host-21_mem.shared.average":                         100,
		"host-21_mem.sharedcommon.average":                   100,
		"host-21_mem.swapinRate.average":                     100,
		"host-21_mem.swapoutRate.average":                    100,
		"host-21_mem.usage.average":                          100,
		"host-21_net.bytesRx.average":                        100,
		"host-21_net.bytesTx.average":                        100,
		"host-21_net.droppedRx.summation":                    100,
		"host-21_net.droppedTx.summation":                    100,
		"host-21_net.errorsRx.summation":                     100,
		"host-21_net.errorsTx.summation":                     100,
		"host-21_net.packetsRx.summation":                    100,
		"host-21_net.packetsTx.summation":                    100,
		"host-21_overall.status.gray":                        1,
		"host-21_overall.status.green":                       0,
		"host-21_overall.status.red":                         0,
		"host-21_overall.status.yellow":                      0,
		"host-21_sys.uptime.latest":                          100,
		"host-37_cpu.usage.average":                          100,
		"host-37_disk.maxTotalLatency.latest":                100,
		"host-37_disk.read.average":                          100,
		"host-37_disk.write.average":                         100,
		"host-37_mem.active.average":                         100,
		"host-37_mem.consumed.average":                       100,
		"host-37_mem.granted.average":                        100,
		"host-37_mem.shared.average":                         100,
		"host-37_mem.sharedcommon.average":                   100,
		"host-37_mem.swapinRate.average":                     100,
		"host-37_mem.swapoutRate.average":                    100,
		"host-37_mem.usage.average":                          100,
		"host-37_net.bytesRx.average":                        100,
		"host-37_net.bytesTx.average":                        100,
		"host-37_net.droppedRx.summation":                    100,
		"host-37_net.droppedTx.summation":                    100,
		"host-37_net.errorsRx.summation":                     100,
		"host-37_net.errorsTx.summation":                     100,
		"host-37_net.packetsRx.summation":                    100,
		"host-37_net.packetsTx.summation":                    100,
		"host-37_overall.status.gray":                        1,
		"host-37_overall.status.green":                       0,
		"host-37_overall.status.red":                         0,
		"host-37_overall.status.yellow":                      0,
		"host-37_sys.uptime.latest":                          100,
		"host-47_cpu.usage.average":                          100,
		"host-47_disk.maxTotalLatency.latest":                100,
		"host-47_disk.read.average":                          100,
		"host-47_disk.write.average":                         100,
		"host-47_mem.active.average":                         100,
		"host-47_mem.consumed.average":                       100,
		"host-47_mem.granted.average":                        100,
		"host-47_mem.shared.average":                         100,
		"host-47_mem.sharedcommon.average":                   100,
		"host-47_mem.swapinRate.average":                     100,
		"host-47_mem.swapoutRate.average":                    100,
		"host-47_mem.usage.average":                          100,
		"host-47_net.bytesRx.average":                        100,
		"host-47_net.bytesTx.average":                        100,
		"host-47_net.droppedRx.summation":                    100,
		"host-47_net.droppedTx.summation":                    100,
		"host-47_net.errorsRx.summation":                     100,
		"host-47_net.errorsTx.summation":                     100,
		"host-47_net.packetsRx.summation":                    100,
		"host-47_net.packetsTx.summation":                    100,
		"host-47_overall.status.gray":                        1,
		"host-47_overall.status.green":                       0,
		"host-47_overall.status.red":                         0,
		"host-47_overall.status.yellow":                      0,
		"host-47_sys.uptime.latest":                          100,
		"host-57_cpu.usage.average":                          100,
		"host-57_disk.maxTotalLatency.latest":                100,
		"host-57_disk.read.average":                          100,
		"host-57_disk.write.average":                         100,
		"host-57_mem.active.average":                         100,
		"host-57_mem.consumed.average":                       100,
		"host-57_mem.granted.average":                        100,
		"host-57_mem.shared.average":                         100,
		"host-57_mem.sharedcommon.average":                   100,
		"host-57_mem.swapinRate.average":                     100,
		"host-57_mem.swapoutRate.average":                    100,
		"host-57_mem.usage.average":                          100,
		"host-57_net.bytesRx.average":                        100,
		"host-57_net.bytesTx.average":                        100,
		"host-57_net.droppedRx.summation":                    100,
		"host-57_net.droppedTx.summation":                    100,
		"host-57_net.errorsRx.summation":                     100,
		"host-57_net.errorsTx.summation":                     100,
		"host-57_net.packetsRx.summation":                    100,
		"host-57_net.packetsTx.summation":                    100,
		"host-57_overall.status.gray":                        1,
		"host-57_overall.status.green":                       0,
		"host-57_overall.status.red":                         0,
		"host-57_overall.status.yellow":                      0,
		"host-57_sys.uptime.latest":                          100,
		"vm-62_cpu.usage.average":                            200,
		"vm-62_disk.maxTotalLatency.latest":                  200,
		"vm-62_disk.read.average":                            200,
		"vm-62_disk.write.average":                           200,
		"vm-62_mem.active.average":                           200,
		"vm-62_mem.consumed.average":                         200,
		"vm-62_mem.granted.average":                          200,
		"vm-62_mem.shared.average":                           200,
		"vm-62_mem.swapinRate.average":                       200,
		"vm-62_mem.swapoutRate.average":                      200,
		"vm-62_mem.swapped.average":                          200,
		"vm-62_mem.usage.average":                            200,
		"vm-62_net.bytesRx.average":                          200,
		"vm-62_net.bytesTx.average":                          200,
		"vm-62_net.droppedRx.summation":                      200,
		"vm-62_net.droppedTx.summation":                      200,
		"vm-62_net.packetsRx.summation":                      200,
		"vm-62_net.packetsTx.summation":                      200,
		"vm-62_overall.status.gray":                          0,
		"vm-62_overall.status.green":                         1,
		"vm-62_overall.status.red":                           0,
		"vm-62_overall.status.yellow":                        0,
		"vm-62_sys.uptime.latest":                            200,
		"vm-65_cpu.usage.average":                            200,
		"vm-65_disk.maxTotalLatency.latest":                  200,
		"vm-65_disk.read.average":                            200,
		"vm-65_disk.write.average":                           200,
		"vm-65_mem.active.average":                           200,
		"vm-65_mem.consumed.average":                         200,
		"vm-65_mem.granted.average":                          200,
		"vm-65_mem.shared.average":                           200,
		"vm-65_mem.swapinRate.average":                       200,
		"vm-65_mem.swapoutRate.average":                      200,
		"vm-65_mem.swapped.average":                          200,
		"vm-65_mem.usage.average":                            200,
		"vm-65_net.bytesRx.average":                          200,
		"vm-65_net.bytesTx.average":                          200,
		"vm-65_net.droppedRx.summation":                      200,
		"vm-65_net.droppedTx.summation":                      200,
		"vm-65_net.packetsRx.summation":                      200,
		"vm-65_net.packetsTx.summation":                      200,
		"vm-65_overall.status.gray":                          0,
		"vm-65_overall.status.green":                         1,
		"vm-65_overall.status.red":                           0,
		"vm-65_overall.status.yellow":                        0,
		"vm-65_sys.uptime.latest":                            200,
		"vm-68_cpu.usage.average":                            200,
		"vm-68_disk.maxTotalLatency.latest":                  200,
		"vm-68_disk.read.average":                            200,
		"vm-68_disk.write.average":                           200,
		"vm-68_mem.active.average":                           200,
		"vm-68_mem.consumed.average":                         200,
		"vm-68_mem.granted.average":                          200,
		"vm-68_mem.shared.average":                           200,
		"vm-68_mem.swapinRate.average":                       200,
		"vm-68_mem.swapoutRate.average":                      200,
		"vm-68_mem.swapped.average":                          200,
		"vm-68_mem.usage.average":                            200,
		"vm-68_net.bytesRx.average":                          200,
		"vm-68_net.bytesTx.average":                          200,
		"vm-68_net.droppedRx.summation":                      200,
		"vm-68_net.droppedTx.summation":                      200,
		"vm-68_net.packetsRx.summation":                      200,
		"vm-68_net.packetsTx.summation":                      200,
		"vm-68_overall.status.gray":                          0,
		"vm-68_overall.status.green":                         1,
		"vm-68_overall.status.red":                           0,
		"vm-68_overall.status.yellow":                        0,
		"vm-68_sys.uptime.latest":                            200,
		"vm-71_cpu.usage.average":                            200,
		"vm-71_disk.maxTotalLatency.latest":                  200,
		"vm-71_disk.read.average":                            200,
		"vm-71_disk.write.average":                           200,
		"vm-71_mem.active.average":                           200,
		"vm-71_mem.consumed.average":                         200,
		"vm-71_mem.granted.average":                          200,
		"vm-71_mem.shared.average":                           200,
		"vm-71_mem.swapinRate.average":                       200,
		"vm-71_mem.swapoutRate.average":                      200,
		"vm-71_mem.swapped.average":                          200,
		"vm-71_mem.usage.average":                            200,
		"vm-71_net.bytesRx.average":                          200,
		"vm-71_net.bytesTx.average":                          200,
		"vm-71_net.droppedRx.summation":                      200,
		"vm-71_net.droppedTx.summation":                      200,
		"vm-71_net.packetsRx.summation":                      200,
		"vm-71_net.packetsTx.summation":                      200,
		"vm-71_overall.status.gray":                          0,
		"vm-71_overall.status.green":                         1,
		"vm-71_overall.status.red":                           0,
		"vm-71_overall.status.yellow":                        0,
		"vm-71_sys.uptime.latest":                            200,
		"datastore-59_capacity":                              4398046511104,
		"datastore-59_free_space":                            4355096838144,
		"datastore-59_used_space":                            42949672960,
		"datastore-59_used_space_pct":                        97,
		"datastore-59_overall.status.green":                  1,
		"datastore-59_overall.status.gray":                   0,
		"datastore-59_overall.status.red":                    0,
		"datastore-59_overall.status.yellow":                 0,
		"datastore-59_datastore.numberReadAveraged.average":  300,
		"datastore-59_datastore.numberWriteAveraged.average": 300,
		"datastore-59_datastore.read.average":                300,
		"datastore-59_datastore.write.average":               300,
		"datastore-59_datastore.totalReadLatency.average":    300,
		"datastore-59_datastore.totalWriteLatency.average":   300,
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

	mx := collr.Collect(context.Background())

	require.Equal(t, expected, mx)

	count := model.Count()
	assert.Len(t, collr.discoveredHosts, count.Host)
	assert.Len(t, collr.discoveredVMs, count.Machine)
	assert.Len(t, collr.discoveredDatastores, count.Datastore)

	numClusters := len(collr.discoveredClusters)
	numResourcePools := len(collr.discoveredResourcePools)
	assert.Len(t, collr.charted, count.Host+count.Machine+count.Datastore+numClusters+numResourcePools)

	assert.Len(t, *collr.Charts(),
		count.Host*len(hostChartsTmpl)+
			count.Machine*len(vmChartsTmpl)+
			count.Datastore*(len(datastorePropertyChartsTmpl)+len(datastorePerfChartsTmpl))+
			numClusters*(len(clusterPropertyChartsTmpl)+len(clusterPerfChartsTmpl))+
			numResourcePools*len(resourcePoolChartsTmpl))
	collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
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
		collr.Collect(context.Background())
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
		collr.Collect(context.Background())
	}

	assert.Len(t, collr.discoveredHosts, 1)
	assert.Len(t, collr.discoveredVMs, 1)
	assert.Len(t, collr.charted, 2+len(collr.discoveredDatastores)+len(collr.discoveredClusters)+len(collr.discoveredResourcePools))

	for _, c := range *collr.Charts() {
		if strings.HasPrefix(c.ID, okHostId) || strings.HasPrefix(c.ID, okVmId) ||
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
		assert.True(t, len(collr.Collect(context.Background())) > 0)
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

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)

	count := model.Count()

	// Property metrics should be present.
	for _, ds := range collr.resources.Datastores {
		assert.Contains(t, mx, ds.ID+"_capacity")
		assert.Contains(t, mx, ds.ID+"_free_space")
		assert.Contains(t, mx, ds.ID+"_used_space")
		assert.Contains(t, mx, ds.ID+"_used_space_pct")
		assert.Contains(t, mx, ds.ID+"_overall.status.green")
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

	mx = collr.Collect(context.Background())
	require.NotNil(t, mx)

	// Perf metrics should now be present.
	for _, ds := range collr.resources.Datastores {
		assert.Contains(t, mx, ds.ID+"_datastore.read.average")
	}

	// Both property and perf charts now for datastores.
	assert.Len(t, *collr.Charts(),
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

	mx := collr.Collect(context.Background())
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

	mx = collr.Collect(context.Background())
	require.NotNil(t, mx)

	// Perf metrics should now be present.
	for _, cl := range collr.resources.Clusters {
		assert.Contains(t, mx, cl.ID+"_cpu.usage.average")
	}

	// Both property and perf charts now for clusters.
	assert.Len(t, *collr.Charts(),
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
	mx := collr.Collect(context.Background())
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
	collr.Collect(context.Background())

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
	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)

	assert.NotEmpty(t, collr.discoveredResourcePools)

	// Simulate eviction: disable property collector so the counter isn't reset.
	collr.rpPropertyCollector = nil
	for id := range collr.discoveredResourcePools {
		collr.discoveredResourcePools[id] = failedUpdatesLimit
	}

	// Next collect increments counter past the limit, triggering eviction.
	collr.Collect(context.Background())

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
	mx := collr.Collect(context.Background())
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
	collr.Collect(context.Background())

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
