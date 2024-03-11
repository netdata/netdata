// SPDX-License-Identifier: GPL-3.0-or-later
package vsphere

import (
	"crypto/tls"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/modules/vsphere/discover"
	"github.com/netdata/netdata/go/go.d.plugin/modules/vsphere/match"
	rs "github.com/netdata/netdata/go/go.d.plugin/modules/vsphere/resources"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"

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

func TestVSphere_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &VSphere{}, dataConfigJSON, dataConfigYAML)
}

func TestVSphere_Init(t *testing.T) {
	vSphere, _, teardown := prepareVSphereSim(t)
	defer teardown()

	assert.NoError(t, vSphere.Init())
	assert.NotNil(t, vSphere.discoverer)
	assert.NotNil(t, vSphere.scraper)
	assert.NotNil(t, vSphere.resources)
	assert.NotNil(t, vSphere.discoveryTask)
	assert.True(t, vSphere.discoveryTask.isRunning())
}

func TestVSphere_Init_ReturnsFalseIfURLNotSet(t *testing.T) {
	vSphere, _, teardown := prepareVSphereSim(t)
	defer teardown()
	vSphere.URL = ""

	assert.Error(t, vSphere.Init())
}

func TestVSphere_Init_ReturnsFalseIfUsernameNotSet(t *testing.T) {
	vSphere, _, teardown := prepareVSphereSim(t)
	defer teardown()
	vSphere.Username = ""

	assert.Error(t, vSphere.Init())
}

func TestVSphere_Init_ReturnsFalseIfPasswordNotSet(t *testing.T) {
	vSphere, _, teardown := prepareVSphereSim(t)
	defer teardown()
	vSphere.Password = ""

	assert.Error(t, vSphere.Init())
}

func TestVSphere_Init_ReturnsFalseIfClientWrongTLSCA(t *testing.T) {
	vSphere, _, teardown := prepareVSphereSim(t)
	defer teardown()
	vSphere.Client.TLSConfig.TLSCA = "testdata/tls"

	assert.Error(t, vSphere.Init())
}

func TestVSphere_Init_ReturnsFalseIfConnectionRefused(t *testing.T) {
	vSphere, _, teardown := prepareVSphereSim(t)
	defer teardown()
	vSphere.URL = "http://127.0.0.1:32001"

	assert.Error(t, vSphere.Init())
}

func TestVSphere_Init_ReturnsFalseIfInvalidHostVMIncludeFormat(t *testing.T) {
	vSphere, _, teardown := prepareVSphereSim(t)
	defer teardown()

	vSphere.HostsInclude = match.HostIncludes{"invalid"}
	assert.Error(t, vSphere.Init())

	vSphere.HostsInclude = vSphere.HostsInclude[:0]

	vSphere.VMsInclude = match.VMIncludes{"invalid"}
	assert.Error(t, vSphere.Init())
}

func TestVSphere_Check(t *testing.T) {
	assert.NoError(t, New().Check())
}

func TestVSphere_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestVSphere_Cleanup(t *testing.T) {
	vSphere, _, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, vSphere.Init())

	vSphere.Cleanup()
	time.Sleep(time.Second)
	assert.True(t, vSphere.discoveryTask.isStopped())
	assert.False(t, vSphere.discoveryTask.isRunning())
}

func TestVSphere_Cleanup_NotPanicsIfNotInitialized(t *testing.T) {
	assert.NotPanics(t, New().Cleanup)
}

func TestVSphere_Collect(t *testing.T) {
	vSphere, model, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, vSphere.Init())

	vSphere.scraper = mockScraper{vSphere.scraper}

	expected := map[string]int64{
		"host-20_cpu.usage.average":           100,
		"host-20_disk.maxTotalLatency.latest": 100,
		"host-20_disk.read.average":           100,
		"host-20_disk.write.average":          100,
		"host-20_mem.active.average":          100,
		"host-20_mem.consumed.average":        100,
		"host-20_mem.granted.average":         100,
		"host-20_mem.shared.average":          100,
		"host-20_mem.sharedcommon.average":    100,
		"host-20_mem.swapinRate.average":      100,
		"host-20_mem.swapoutRate.average":     100,
		"host-20_mem.usage.average":           100,
		"host-20_net.bytesRx.average":         100,
		"host-20_net.bytesTx.average":         100,
		"host-20_net.droppedRx.summation":     100,
		"host-20_net.droppedTx.summation":     100,
		"host-20_net.errorsRx.summation":      100,
		"host-20_net.errorsTx.summation":      100,
		"host-20_net.packetsRx.summation":     100,
		"host-20_net.packetsTx.summation":     100,
		"host-20_overall.status.gray":         1,
		"host-20_overall.status.green":        0,
		"host-20_overall.status.red":          0,
		"host-20_overall.status.yellow":       0,
		"host-20_sys.uptime.latest":           100,
		"host-34_cpu.usage.average":           100,
		"host-34_disk.maxTotalLatency.latest": 100,
		"host-34_disk.read.average":           100,
		"host-34_disk.write.average":          100,
		"host-34_mem.active.average":          100,
		"host-34_mem.consumed.average":        100,
		"host-34_mem.granted.average":         100,
		"host-34_mem.shared.average":          100,
		"host-34_mem.sharedcommon.average":    100,
		"host-34_mem.swapinRate.average":      100,
		"host-34_mem.swapoutRate.average":     100,
		"host-34_mem.usage.average":           100,
		"host-34_net.bytesRx.average":         100,
		"host-34_net.bytesTx.average":         100,
		"host-34_net.droppedRx.summation":     100,
		"host-34_net.droppedTx.summation":     100,
		"host-34_net.errorsRx.summation":      100,
		"host-34_net.errorsTx.summation":      100,
		"host-34_net.packetsRx.summation":     100,
		"host-34_net.packetsTx.summation":     100,
		"host-34_overall.status.gray":         1,
		"host-34_overall.status.green":        0,
		"host-34_overall.status.red":          0,
		"host-34_overall.status.yellow":       0,
		"host-34_sys.uptime.latest":           100,
		"host-42_cpu.usage.average":           100,
		"host-42_disk.maxTotalLatency.latest": 100,
		"host-42_disk.read.average":           100,
		"host-42_disk.write.average":          100,
		"host-42_mem.active.average":          100,
		"host-42_mem.consumed.average":        100,
		"host-42_mem.granted.average":         100,
		"host-42_mem.shared.average":          100,
		"host-42_mem.sharedcommon.average":    100,
		"host-42_mem.swapinRate.average":      100,
		"host-42_mem.swapoutRate.average":     100,
		"host-42_mem.usage.average":           100,
		"host-42_net.bytesRx.average":         100,
		"host-42_net.bytesTx.average":         100,
		"host-42_net.droppedRx.summation":     100,
		"host-42_net.droppedTx.summation":     100,
		"host-42_net.errorsRx.summation":      100,
		"host-42_net.errorsTx.summation":      100,
		"host-42_net.packetsRx.summation":     100,
		"host-42_net.packetsTx.summation":     100,
		"host-42_overall.status.gray":         1,
		"host-42_overall.status.green":        0,
		"host-42_overall.status.red":          0,
		"host-42_overall.status.yellow":       0,
		"host-42_sys.uptime.latest":           100,
		"host-50_cpu.usage.average":           100,
		"host-50_disk.maxTotalLatency.latest": 100,
		"host-50_disk.read.average":           100,
		"host-50_disk.write.average":          100,
		"host-50_mem.active.average":          100,
		"host-50_mem.consumed.average":        100,
		"host-50_mem.granted.average":         100,
		"host-50_mem.shared.average":          100,
		"host-50_mem.sharedcommon.average":    100,
		"host-50_mem.swapinRate.average":      100,
		"host-50_mem.swapoutRate.average":     100,
		"host-50_mem.usage.average":           100,
		"host-50_net.bytesRx.average":         100,
		"host-50_net.bytesTx.average":         100,
		"host-50_net.droppedRx.summation":     100,
		"host-50_net.droppedTx.summation":     100,
		"host-50_net.errorsRx.summation":      100,
		"host-50_net.errorsTx.summation":      100,
		"host-50_net.packetsRx.summation":     100,
		"host-50_net.packetsTx.summation":     100,
		"host-50_overall.status.gray":         1,
		"host-50_overall.status.green":        0,
		"host-50_overall.status.red":          0,
		"host-50_overall.status.yellow":       0,
		"host-50_sys.uptime.latest":           100,
		"vm-55_cpu.usage.average":             200,
		"vm-55_disk.maxTotalLatency.latest":   200,
		"vm-55_disk.read.average":             200,
		"vm-55_disk.write.average":            200,
		"vm-55_mem.active.average":            200,
		"vm-55_mem.consumed.average":          200,
		"vm-55_mem.granted.average":           200,
		"vm-55_mem.shared.average":            200,
		"vm-55_mem.swapinRate.average":        200,
		"vm-55_mem.swapoutRate.average":       200,
		"vm-55_mem.swapped.average":           200,
		"vm-55_mem.usage.average":             200,
		"vm-55_net.bytesRx.average":           200,
		"vm-55_net.bytesTx.average":           200,
		"vm-55_net.droppedRx.summation":       200,
		"vm-55_net.droppedTx.summation":       200,
		"vm-55_net.packetsRx.summation":       200,
		"vm-55_net.packetsTx.summation":       200,
		"vm-55_overall.status.gray":           0,
		"vm-55_overall.status.green":          1,
		"vm-55_overall.status.red":            0,
		"vm-55_overall.status.yellow":         0,
		"vm-55_sys.uptime.latest":             200,
		"vm-58_cpu.usage.average":             200,
		"vm-58_disk.maxTotalLatency.latest":   200,
		"vm-58_disk.read.average":             200,
		"vm-58_disk.write.average":            200,
		"vm-58_mem.active.average":            200,
		"vm-58_mem.consumed.average":          200,
		"vm-58_mem.granted.average":           200,
		"vm-58_mem.shared.average":            200,
		"vm-58_mem.swapinRate.average":        200,
		"vm-58_mem.swapoutRate.average":       200,
		"vm-58_mem.swapped.average":           200,
		"vm-58_mem.usage.average":             200,
		"vm-58_net.bytesRx.average":           200,
		"vm-58_net.bytesTx.average":           200,
		"vm-58_net.droppedRx.summation":       200,
		"vm-58_net.droppedTx.summation":       200,
		"vm-58_net.packetsRx.summation":       200,
		"vm-58_net.packetsTx.summation":       200,
		"vm-58_overall.status.gray":           0,
		"vm-58_overall.status.green":          1,
		"vm-58_overall.status.red":            0,
		"vm-58_overall.status.yellow":         0,
		"vm-58_sys.uptime.latest":             200,
		"vm-61_cpu.usage.average":             200,
		"vm-61_disk.maxTotalLatency.latest":   200,
		"vm-61_disk.read.average":             200,
		"vm-61_disk.write.average":            200,
		"vm-61_mem.active.average":            200,
		"vm-61_mem.consumed.average":          200,
		"vm-61_mem.granted.average":           200,
		"vm-61_mem.shared.average":            200,
		"vm-61_mem.swapinRate.average":        200,
		"vm-61_mem.swapoutRate.average":       200,
		"vm-61_mem.swapped.average":           200,
		"vm-61_mem.usage.average":             200,
		"vm-61_net.bytesRx.average":           200,
		"vm-61_net.bytesTx.average":           200,
		"vm-61_net.droppedRx.summation":       200,
		"vm-61_net.droppedTx.summation":       200,
		"vm-61_net.packetsRx.summation":       200,
		"vm-61_net.packetsTx.summation":       200,
		"vm-61_overall.status.gray":           0,
		"vm-61_overall.status.green":          1,
		"vm-61_overall.status.red":            0,
		"vm-61_overall.status.yellow":         0,
		"vm-61_sys.uptime.latest":             200,
		"vm-64_cpu.usage.average":             200,
		"vm-64_disk.maxTotalLatency.latest":   200,
		"vm-64_disk.read.average":             200,
		"vm-64_disk.write.average":            200,
		"vm-64_mem.active.average":            200,
		"vm-64_mem.consumed.average":          200,
		"vm-64_mem.granted.average":           200,
		"vm-64_mem.shared.average":            200,
		"vm-64_mem.swapinRate.average":        200,
		"vm-64_mem.swapoutRate.average":       200,
		"vm-64_mem.swapped.average":           200,
		"vm-64_mem.usage.average":             200,
		"vm-64_net.bytesRx.average":           200,
		"vm-64_net.bytesTx.average":           200,
		"vm-64_net.droppedRx.summation":       200,
		"vm-64_net.droppedTx.summation":       200,
		"vm-64_net.packetsRx.summation":       200,
		"vm-64_net.packetsTx.summation":       200,
		"vm-64_overall.status.gray":           0,
		"vm-64_overall.status.green":          1,
		"vm-64_overall.status.red":            0,
		"vm-64_overall.status.yellow":         0,
		"vm-64_sys.uptime.latest":             200,
	}

	collected := vSphere.Collect()
	require.Equal(t, expected, collected)

	count := model.Count()
	assert.Len(t, vSphere.discoveredHosts, count.Host)
	assert.Len(t, vSphere.discoveredVMs, count.Machine)
	assert.Len(t, vSphere.charted, count.Host+count.Machine)

	assert.Len(t, *vSphere.Charts(), count.Host*len(hostChartsTmpl)+count.Machine*len(vmChartsTmpl))
	ensureCollectedHasAllChartsDimsVarsIDs(t, vSphere, collected)
}

func TestVSphere_Collect_RemoveHostsVMsInRuntime(t *testing.T) {
	vSphere, _, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, vSphere.Init())
	require.NoError(t, vSphere.Check())

	okHostID := "host-50"
	okVMID := "vm-64"
	vSphere.discoverer.(*discover.Discoverer).HostMatcher = mockHostMatcher{okHostID}
	vSphere.discoverer.(*discover.Discoverer).VMMatcher = mockVMMatcher{okVMID}

	require.NoError(t, vSphere.discoverOnce())

	numOfRuns := 5
	for i := 0; i < numOfRuns; i++ {
		vSphere.Collect()
	}

	host := vSphere.resources.Hosts.Get(okHostID)
	for k, v := range vSphere.discoveredHosts {
		if k == host.ID {
			assert.Equal(t, 0, v)
		} else {
			assert.Equal(t, numOfRuns, v)
		}
	}

	vm := vSphere.resources.VMs.Get(okVMID)
	for id, fails := range vSphere.discoveredVMs {
		if id == vm.ID {
			assert.Equal(t, 0, fails)
		} else {
			assert.Equal(t, numOfRuns, fails)
		}

	}

	for i := numOfRuns; i < failedUpdatesLimit; i++ {
		vSphere.Collect()
	}

	assert.Len(t, vSphere.discoveredHosts, 1)
	assert.Len(t, vSphere.discoveredVMs, 1)
	assert.Len(t, vSphere.charted, 2)

	for _, c := range *vSphere.Charts() {
		if strings.HasPrefix(c.ID, okHostID) || strings.HasPrefix(c.ID, okVMID) {
			assert.False(t, c.Obsolete)
		} else {
			assert.True(t, c.Obsolete)
		}
	}
}

func TestVSphere_Collect_Run(t *testing.T) {
	vSphere, model, teardown := prepareVSphereSim(t)
	defer teardown()

	vSphere.DiscoveryInterval = web.Duration(time.Second * 2)
	require.NoError(t, vSphere.Init())
	require.NoError(t, vSphere.Check())

	runs := 20
	for i := 0; i < runs; i++ {
		assert.True(t, len(vSphere.Collect()) > 0)
		if i < 6 {
			time.Sleep(time.Second)
		}
	}

	count := model.Count()
	assert.Len(t, vSphere.discoveredHosts, count.Host)
	assert.Len(t, vSphere.discoveredVMs, count.Machine)
	assert.Len(t, vSphere.charted, count.Host+count.Machine)
	assert.Len(t, *vSphere.charts, count.Host*len(hostChartsTmpl)+count.Machine*len(vmChartsTmpl))
}

func ensureCollectedHasAllChartsDimsVarsIDs(t *testing.T, vSphere *VSphere, collected map[string]int64) {
	for _, chart := range *vSphere.Charts() {
		for _, dim := range chart.Dims {
			_, ok := collected[dim.ID]
			assert.Truef(t, ok, "collected metrics has no data for dim '%s' chart '%s'", dim.ID, chart.ID)
		}
		for _, v := range chart.Vars {
			_, ok := collected[v.ID]
			assert.Truef(t, ok, "collected metrics has no data for var '%s' chart '%s'", v.ID, chart.ID)
		}
	}
}

func prepareVSphereSim(t *testing.T) (vSphere *VSphere, model *simulator.Model, teardown func()) {
	model, srv := createSim(t)
	vSphere = New()
	teardown = func() { model.Remove(); srv.Close(); vSphere.Cleanup() }

	vSphere.Username = "administrator"
	vSphere.Password = "password"
	vSphere.URL = srv.URL.String()
	vSphere.TLSConfig.InsecureSkipVerify = true

	return vSphere, model, teardown
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
