// SPDX-License-Identifier: GPL-3.0-or-later
package vsphere

import (
	"context"
	"crypto/tls"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/discover"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/match"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware/govmomi/performance"
	"github.com/vmware/govmomi/simulator"
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
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
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
		"host-21_cpu.usage.average":           100,
		"host-21_disk.maxTotalLatency.latest": 100,
		"host-21_disk.read.average":           100,
		"host-21_disk.write.average":          100,
		"host-21_mem.active.average":          100,
		"host-21_mem.consumed.average":        100,
		"host-21_mem.granted.average":         100,
		"host-21_mem.shared.average":          100,
		"host-21_mem.sharedcommon.average":    100,
		"host-21_mem.swapinRate.average":      100,
		"host-21_mem.swapoutRate.average":     100,
		"host-21_mem.usage.average":           100,
		"host-21_net.bytesRx.average":         100,
		"host-21_net.bytesTx.average":         100,
		"host-21_net.droppedRx.summation":     100,
		"host-21_net.droppedTx.summation":     100,
		"host-21_net.errorsRx.summation":      100,
		"host-21_net.errorsTx.summation":      100,
		"host-21_net.packetsRx.summation":     100,
		"host-21_net.packetsTx.summation":     100,
		"host-21_overall.status.gray":         1,
		"host-21_overall.status.green":        0,
		"host-21_overall.status.red":          0,
		"host-21_overall.status.yellow":       0,
		"host-21_sys.uptime.latest":           100,
		"host-37_cpu.usage.average":           100,
		"host-37_disk.maxTotalLatency.latest": 100,
		"host-37_disk.read.average":           100,
		"host-37_disk.write.average":          100,
		"host-37_mem.active.average":          100,
		"host-37_mem.consumed.average":        100,
		"host-37_mem.granted.average":         100,
		"host-37_mem.shared.average":          100,
		"host-37_mem.sharedcommon.average":    100,
		"host-37_mem.swapinRate.average":      100,
		"host-37_mem.swapoutRate.average":     100,
		"host-37_mem.usage.average":           100,
		"host-37_net.bytesRx.average":         100,
		"host-37_net.bytesTx.average":         100,
		"host-37_net.droppedRx.summation":     100,
		"host-37_net.droppedTx.summation":     100,
		"host-37_net.errorsRx.summation":      100,
		"host-37_net.errorsTx.summation":      100,
		"host-37_net.packetsRx.summation":     100,
		"host-37_net.packetsTx.summation":     100,
		"host-37_overall.status.gray":         1,
		"host-37_overall.status.green":        0,
		"host-37_overall.status.red":          0,
		"host-37_overall.status.yellow":       0,
		"host-37_sys.uptime.latest":           100,
		"host-47_cpu.usage.average":           100,
		"host-47_disk.maxTotalLatency.latest": 100,
		"host-47_disk.read.average":           100,
		"host-47_disk.write.average":          100,
		"host-47_mem.active.average":          100,
		"host-47_mem.consumed.average":        100,
		"host-47_mem.granted.average":         100,
		"host-47_mem.shared.average":          100,
		"host-47_mem.sharedcommon.average":    100,
		"host-47_mem.swapinRate.average":      100,
		"host-47_mem.swapoutRate.average":     100,
		"host-47_mem.usage.average":           100,
		"host-47_net.bytesRx.average":         100,
		"host-47_net.bytesTx.average":         100,
		"host-47_net.droppedRx.summation":     100,
		"host-47_net.droppedTx.summation":     100,
		"host-47_net.errorsRx.summation":      100,
		"host-47_net.errorsTx.summation":      100,
		"host-47_net.packetsRx.summation":     100,
		"host-47_net.packetsTx.summation":     100,
		"host-47_overall.status.gray":         1,
		"host-47_overall.status.green":        0,
		"host-47_overall.status.red":          0,
		"host-47_overall.status.yellow":       0,
		"host-47_sys.uptime.latest":           100,
		"host-57_cpu.usage.average":           100,
		"host-57_disk.maxTotalLatency.latest": 100,
		"host-57_disk.read.average":           100,
		"host-57_disk.write.average":          100,
		"host-57_mem.active.average":          100,
		"host-57_mem.consumed.average":        100,
		"host-57_mem.granted.average":         100,
		"host-57_mem.shared.average":          100,
		"host-57_mem.sharedcommon.average":    100,
		"host-57_mem.swapinRate.average":      100,
		"host-57_mem.swapoutRate.average":     100,
		"host-57_mem.usage.average":           100,
		"host-57_net.bytesRx.average":         100,
		"host-57_net.bytesTx.average":         100,
		"host-57_net.droppedRx.summation":     100,
		"host-57_net.droppedTx.summation":     100,
		"host-57_net.errorsRx.summation":      100,
		"host-57_net.errorsTx.summation":      100,
		"host-57_net.packetsRx.summation":     100,
		"host-57_net.packetsTx.summation":     100,
		"host-57_overall.status.gray":         1,
		"host-57_overall.status.green":        0,
		"host-57_overall.status.red":          0,
		"host-57_overall.status.yellow":       0,
		"host-57_sys.uptime.latest":           100,
		"vm-62_cpu.usage.average":             200,
		"vm-62_disk.maxTotalLatency.latest":   200,
		"vm-62_disk.read.average":             200,
		"vm-62_disk.write.average":            200,
		"vm-62_mem.active.average":            200,
		"vm-62_mem.consumed.average":          200,
		"vm-62_mem.granted.average":           200,
		"vm-62_mem.shared.average":            200,
		"vm-62_mem.swapinRate.average":        200,
		"vm-62_mem.swapoutRate.average":       200,
		"vm-62_mem.swapped.average":           200,
		"vm-62_mem.usage.average":             200,
		"vm-62_net.bytesRx.average":           200,
		"vm-62_net.bytesTx.average":           200,
		"vm-62_net.droppedRx.summation":       200,
		"vm-62_net.droppedTx.summation":       200,
		"vm-62_net.packetsRx.summation":       200,
		"vm-62_net.packetsTx.summation":       200,
		"vm-62_overall.status.gray":           0,
		"vm-62_overall.status.green":          1,
		"vm-62_overall.status.red":            0,
		"vm-62_overall.status.yellow":         0,
		"vm-62_sys.uptime.latest":             200,
		"vm-65_cpu.usage.average":             200,
		"vm-65_disk.maxTotalLatency.latest":   200,
		"vm-65_disk.read.average":             200,
		"vm-65_disk.write.average":            200,
		"vm-65_mem.active.average":            200,
		"vm-65_mem.consumed.average":          200,
		"vm-65_mem.granted.average":           200,
		"vm-65_mem.shared.average":            200,
		"vm-65_mem.swapinRate.average":        200,
		"vm-65_mem.swapoutRate.average":       200,
		"vm-65_mem.swapped.average":           200,
		"vm-65_mem.usage.average":             200,
		"vm-65_net.bytesRx.average":           200,
		"vm-65_net.bytesTx.average":           200,
		"vm-65_net.droppedRx.summation":       200,
		"vm-65_net.droppedTx.summation":       200,
		"vm-65_net.packetsRx.summation":       200,
		"vm-65_net.packetsTx.summation":       200,
		"vm-65_overall.status.gray":           0,
		"vm-65_overall.status.green":          1,
		"vm-65_overall.status.red":            0,
		"vm-65_overall.status.yellow":         0,
		"vm-65_sys.uptime.latest":             200,
		"vm-68_cpu.usage.average":             200,
		"vm-68_disk.maxTotalLatency.latest":   200,
		"vm-68_disk.read.average":             200,
		"vm-68_disk.write.average":            200,
		"vm-68_mem.active.average":            200,
		"vm-68_mem.consumed.average":          200,
		"vm-68_mem.granted.average":           200,
		"vm-68_mem.shared.average":            200,
		"vm-68_mem.swapinRate.average":        200,
		"vm-68_mem.swapoutRate.average":       200,
		"vm-68_mem.swapped.average":           200,
		"vm-68_mem.usage.average":             200,
		"vm-68_net.bytesRx.average":           200,
		"vm-68_net.bytesTx.average":           200,
		"vm-68_net.droppedRx.summation":       200,
		"vm-68_net.droppedTx.summation":       200,
		"vm-68_net.packetsRx.summation":       200,
		"vm-68_net.packetsTx.summation":       200,
		"vm-68_overall.status.gray":           0,
		"vm-68_overall.status.green":          1,
		"vm-68_overall.status.red":            0,
		"vm-68_overall.status.yellow":         0,
		"vm-68_sys.uptime.latest":             200,
		"vm-71_cpu.usage.average":             200,
		"vm-71_disk.maxTotalLatency.latest":   200,
		"vm-71_disk.read.average":             200,
		"vm-71_disk.write.average":            200,
		"vm-71_mem.active.average":            200,
		"vm-71_mem.consumed.average":          200,
		"vm-71_mem.granted.average":           200,
		"vm-71_mem.shared.average":            200,
		"vm-71_mem.swapinRate.average":        200,
		"vm-71_mem.swapoutRate.average":       200,
		"vm-71_mem.swapped.average":           200,
		"vm-71_mem.usage.average":             200,
		"vm-71_net.bytesRx.average":           200,
		"vm-71_net.bytesTx.average":           200,
		"vm-71_net.droppedRx.summation":       200,
		"vm-71_net.droppedTx.summation":       200,
		"vm-71_net.packetsRx.summation":       200,
		"vm-71_net.packetsTx.summation":       200,
		"vm-71_overall.status.gray":           0,
		"vm-71_overall.status.green":          1,
		"vm-71_overall.status.red":            0,
		"vm-71_overall.status.yellow":         0,
		"vm-71_sys.uptime.latest":             200,
	}

	mx := collr.Collect(context.Background())

	require.Equal(t, expected, mx)

	count := model.Count()
	assert.Len(t, collr.discoveredHosts, count.Host)
	assert.Len(t, collr.discoveredVMs, count.Machine)
	assert.Len(t, collr.charted, count.Host+count.Machine)

	assert.Len(t, *collr.Charts(), count.Host*len(hostChartsTmpl)+count.Machine*len(vmChartsTmpl))
	module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
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
	for i := 0; i < numOfRuns; i++ {
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
	assert.Len(t, collr.charted, 2)

	for _, c := range *collr.Charts() {
		if strings.HasPrefix(c.ID, okHostId) || strings.HasPrefix(c.ID, okVmId) {
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
	for i := 0; i < runs; i++ {
		assert.True(t, len(collr.Collect(context.Background())) > 0)
		if i < 6 {
			time.Sleep(time.Second)
		}
	}

	count := model.Count()
	assert.Len(t, collr.discoveredHosts, count.Host)
	assert.Len(t, collr.discoveredVMs, count.Machine)
	assert.Len(t, collr.charted, count.Host+count.Machine)
	assert.Len(t, *collr.charts, count.Host*len(hostChartsTmpl)+count.Machine*len(vmChartsTmpl))
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
