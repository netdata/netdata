// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"context"
	"errors"
	"maps"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

type wireOutput chan string

func (w wireOutput) Write(p []byte) (int, error) {
	select {
	case w <- string(p):
	default:
	}
	return len(p), nil
}

// timedQueryResults are valid results whose processors have spent a quarter
// of the time since the epoch in user mode and the rest idle.
func timedQueryResults() map[string]queryResult {
	results := validQueryResults()
	now := uint64(time.Now().UnixNano() / 100)
	user, idle := now/4, now/4*3
	results[queryCPU] = fixtureRows(
		processorRow("_Total", 0, user, 0, 0, idle),
		processorRow("0,0", 0, user, 0, 0, idle),
		processorRow("0,1", 0, user, 0, 0, idle),
	)
	// Uplink and the virtual Team Member report every counter, the other
	// interfaces only their byte totals.
	interfaces := slices.Concat(
		results[queryNetworkInterfaces].rows,
		fixtureRows(nativeInterfaceRow("Uplink", 1000)).rows,
	)
	results[queryNetworkInterfaces] = queryResult{
		rows: interfaces,
	}
	results[queryNetworkAdapters] = queryResult{
		rows: slices.Concat(interfaces, fixtureRows(nativeInterfaceRow("Team Member", 2000)).rows),
	}
	disk0, disk1 := timedDiskRow("0 C:", 1), timedDiskRow("1", 2)
	results[queryPhysicalDisks] = fixtureRows(disk0, disk1, map[string]any{
		"Name":                 "_Total",
		"DiskReadBytesPersec":  disk0["DiskReadBytesPersec"].(uint64) + disk1["DiskReadBytesPersec"].(uint64),
		"DiskWriteBytesPersec": disk0["DiskWriteBytesPersec"].(uint64) + disk1["DiskWriteBytesPersec"].(uint64),
	})
	return results
}

// timedDiskRow is a PhysicalDisk instance whose counters have grown at fixed
// rates since the host started a day ago: per second, 20,000 reads of
// scale * 4 KiB taking scale ms each, 2,000 writes of scale * 64 KiB taking
// scale * 4 ms each and 7 split I/Os, with the disk idle 3/5 of the time at
// scale 1 and 1/5 at scale 2. The performance counter runs at 10 MHz, so each
// 32-bit average timer is the low half of its 100 ns queue-length integral. In
// a 30-second interval the read time wraps it more than once, and the read,
// write and total times differ by more than 2^31 ticks, so each timer needs its
// own integral to count its wraps.
func timedDiskRow(name string, scale uint64) map[string]any {
	uptime := uint64(time.Now().Unix()%1000000) + 86400
	reads, writes := 20000*uptime, 2000*uptime
	readTime, writeTime := reads*10000*scale, writes*40000*scale
	idleFifths := map[uint64]uint64{1: 3, 2: 1}[scale]
	return map[string]any{
		"Name":                       name,
		"DiskReadBytesPersec":        reads * 4096 * scale,
		"DiskWriteBytesPersec":       writes * 65536 * scale,
		"DiskReadsPersec":            uint32(reads),
		"DiskWritesPersec":           uint32(writes),
		"SplitIOPerSec":              uint32(7 * uptime),
		"CurrentDiskQueueLength":     uint32(3),
		"PercentDiskTime":            readTime + writeTime,
		"PercentDiskReadTime":        readTime,
		"PercentDiskWriteTime":       writeTime,
		"PercentIdleTime":            uptime * 10000000 * idleFifths / 5,
		"PercentIdleTime_Base":       133000000000000000 + uptime*10000000,
		"AvgDisksecPerRead":          uint32(readTime),
		"AvgDisksecPerRead_Base":     uint32(reads),
		"AvgDiskReadQueueLength":     readTime,
		"AvgDisksecPerWrite":         uint32(writeTime),
		"AvgDisksecPerWrite_Base":    uint32(writes),
		"AvgDiskWriteQueueLength":    writeTime,
		"AvgDisksecPerTransfer":      uint32(readTime + writeTime),
		"AvgDisksecPerTransfer_Base": uint32(reads + writes),
		"AvgDiskQueueLength":         readTime + writeTime,
		"AvgDiskBytesPerRead":        reads * 4096 * scale,
		"AvgDiskBytesPerRead_Base":   uint32(reads),
		"AvgDiskBytesPerWrite":       writes * 65536 * scale,
		"AvgDiskBytesPerWrite_Base":  uint32(writes),
		"Frequency_PerfTime":         uint64(10000000),
	}
}

type nativeDimension struct {
	context   string
	dimension string
}

// nativeInterfaceCounters maps the WMI property of each counter the native
// Windows Agent reads for an interface to the chart context and dimension it
// feeds there (src/collectors/windows.plugin/perflib-network.c,
// network_interface_init and do_network_interface).
var nativeInterfaceCounters = map[string]nativeDimension{
	"BytesReceivedPersec":          {"net.net", "received"},
	"BytesSentPersec":              {"net.net", "sent"},
	"PacketsReceivedPersec":        {"net.packets", "received"},
	"PacketsSentPersec":            {"net.packets", "sent"},
	"CurrentBandwidth":             {"net.speed", "speed"},
	"PacketsReceivedErrors":        {"net.errors", "inbound"},
	"PacketsOutboundErrors":        {"net.errors", "outbound"},
	"PacketsReceivedDiscarded":     {"net.drops", "inbound"},
	"PacketsOutboundDiscarded":     {"net.drops", "outbound"},
	"PacketsReceivedUnknown":       {"net.unknown_protocol_packets", "inbound"},
	"OutputQueueLength":            {"net.queue_length", "length"},
	"TCPActiveRSCConnections":      {"net.rsc_connections", "connections"},
	"TCPRSCCoalescedPacketsPersec": {"net.rsc_packets", "packets"},
	"TCPRSCExceptionsPersec":       {"net.rsc_exceptions", "exceptions"},
	"TCPRSCAveragePacketSize":      {"net.rsc_average_packet_size", "average"},
	"OffloadedConnections":         {"net.chimney_connections", "connections"},
}

// nativeInterfaceRow gives every counter of nativeInterfaceCounters a distinct
// value from base up, in property name order.
func nativeInterfaceRow(name string, base uint64) map[string]any {
	row := map[string]any{"Name": name}
	for i, property := range slices.Sorted(maps.Keys(nativeInterfaceCounters)) {
		row[property] = base + uint64(i)
	}
	return row
}

// startJob runs the collector as a V2 job, inside a synctest bubble, and
// returns a function that ticks the job and returns that tick's output.
func startJob(t *testing.T, c *Collector, vnode vnodes.VirtualNode) func(clock int) string {
	t.Helper()
	out := make(wireOutput, 1024)
	job := jobruntime.NewJobV2(jobruntime.JobV2Config{
		PluginName:  "wmi",
		Name:        "fixture",
		ModuleName:  "windows",
		FullName:    "windows_fixture",
		Module:      c,
		Out:         out,
		UpdateEvery: c.UpdateEvery,
		Vnode:       vnode,
		VnodeName:   vnode.Name,
	})
	require.NoError(t, job.AutoDetectionManaged(t.Context()))
	assert.Equal(t, 0, job.AutoDetectionEvery())

	run := jobruntime.NewManagedRun(t.Context(), nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		job.StartManaged(run)
	}()
	t.Cleanup(func() {
		run.Stop(nil)
		<-done
		job.Cleanup()
	})
	<-run.StartupDone()
	require.NoError(t, run.StartupErr())

	return func(clock int) string {
		t.Helper()
		synctest.Wait()
		job.Tick(clock)
		synctest.Wait()
		var output strings.Builder
		for len(out) > 0 {
			output.WriteString(<-out)
		}
		return output.String()
	}
}

func TestCollector_JobV2Publication(t *testing.T) {
	const namedGUID = "245dea48-ae2a-4697-a22e-050e7e358afb"
	namedStaleAfter := confopt.Duration(5 * time.Minute)
	for name, vnode := range map[string]vnodes.VirtualNode{
		"generated vnode": {},
		"named vnode": {
			Name:     "override",
			Hostname: "configured.example.com",
			GUID:     namedGUID,
		},
		"named vnode with its own stale_after": {
			Name:       "override",
			Hostname:   "configured.example.com",
			GUID:       namedGUID,
			StaleAfter: &namedStaleAfter,
		},
	} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				c := configuredCollector()
				renameAt := time.Now().Add(inventoryInterval)
				c.Vnode = vnode.Name
				c.newSession = func(context.Context) (querySession, error) {
					results := timedQueryResults()
					hostname := "WIN-LAB"
					if !time.Now().Before(renameAt) {
						hostname = "WIN-RENAMED"
					}
					results[queryOSInfo] = fixtureRows(
						map[string]any{
							"CSName":      hostname,
							"Caption":     "Microsoft Windows Server 2022 Standard",
							"ProductType": uint32(3),
						},
					)
					return &fakeSession{
						results: results,
					}, nil
				}
				collect := startJob(t, c, vnode)

				output := collect(c.UpdateEvery)
				assert.Contains(t, output, "SET 'received' = 190")
				assert.Contains(t, output, "CLABEL 'device' 'Ethernet'")
				assert.NotContains(
					t,
					output,
					"SET 'user'",
					"the first interval starts at the readiness check, at the same instant",
				)
				assert.Contains(t, output, "SET 'used' = 6442450944")
				if vnode.Name != "" {
					assert.Contains(t, output, "HOST_DEFINE '"+namedGUID+"' 'configured.example.com'")
					assert.NotContains(t, output, c.VirtualNode().GUID)
					assert.NotContains(t, output, "HOST_LABEL '_os_edition'")
					// A named node keeps its own lifecycle setting.
					if vnode.StaleAfter != nil {
						assert.Contains(t, output, "HOST_LABEL '_node_stale_after_seconds' '300'")
					} else {
						assert.NotContains(t, output, "_node_stale_after_seconds")
					}
					return
				}
				guid := c.VirtualNode().GUID
				assert.Contains(t, output, "HOST_DEFINE '"+guid+"' 'WIN-LAB'")
				assert.Contains(t, output, "HOST_LABEL '_vnode_type' 'wmi'")
				assert.Contains(t, output, "HOST_LABEL 'address' 'windows.example.com'")
				assert.Contains(t, output, "HOST_LABEL '_os_edition' 'Standard'")
				// Three missed 30 s collections plus the 15 s timeout.
				assert.Contains(t, output, "HOST_LABEL '_node_stale_after_seconds' '105'")

				time.Sleep(inventoryInterval)
				renamed := collect(2 * c.UpdateEvery)
				assert.Contains(t, renamed, "HOST_DEFINE '"+guid+"' 'WIN-RENAMED'")
				assert.Contains(t, renamed, "SET 'user' = 25")
				assert.Contains(t, renamed, "HOST_LABEL 'address' 'windows.example.com'")
				assert.Contains(t, renamed, "HOST_LABEL '_node_stale_after_seconds' '105'")
			})
		})
	}
}

// The Agent marks a node stale when it is not selected for longer than its
// timeout, while the plugin keeps selecting localhost. So a failed collection
// must not touch the node but must still report the job status on localhost,
// and the next successful collection selects the node again.
func TestCollector_JobV2HostStatus(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := configuredCollector()
		reachable := true
		c.newSession = func(context.Context) (querySession, error) {
			if !reachable {
				return nil, errors.New("connection timed out")
			}
			return &fakeSession{
				results: timedQueryResults(),
			}, nil
		}
		collect := startJob(t, c, vnodes.VirtualNode{})
		guid := c.VirtualNode().GUID

		up := collect(c.UpdateEvery)
		assert.Contains(t, up, "HOST_DEFINE '"+guid+"'")
		assert.Contains(t, up, "HOST_LABEL '_node_stale_after_seconds' '105'")

		reachable = false
		for clock := 2 * c.UpdateEvery; clock <= 4*c.UpdateEvery; clock += c.UpdateEvery {
			down := collect(clock)
			assert.NotContains(t, down, guid, "clock %d", clock)
			assert.Contains(t, down, "HOST ''", "clock %d", clock)
			assert.Contains(t, down, "SET 'failed' = 1", "clock %d", clock)
		}

		reachable = true
		back := collect(5 * c.UpdateEvery)
		assert.Contains(t, back, "HOST '"+guid+"'")
		assert.Contains(t, back, "SET 'used' = 6442450944")
		assert.Contains(t, back, "SET 'success' = 1")
	})
}

type wireDimension struct {
	name       string
	algorithm  string
	multiplier int
	divisor    int
	hidden     bool
}

type wireChart struct {
	context    string
	family     string
	title      string
	units      string
	chartType  string
	labels     map[string]string
	dimensions []wireDimension
	// variables are the chart variables the definition declares.
	variables []string
}

var protocolField = regexp.MustCompile(`'([^']*)'|(\S+)`)

// parseCharts reads the chart definitions from plugin protocol output, by
// chart ID. Labels the Agent's protocol reserves (a leading underscore) are
// left out.
func parseCharts(t *testing.T, output string) map[string]*wireChart {
	t.Helper()
	charts := make(map[string]*wireChart)
	var current *wireChart
	for line := range strings.Lines(output) {
		var fields []string
		for _, match := range protocolField.FindAllStringSubmatch(line, -1) {
			fields = append(fields, match[1]+match[2])
		}
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "CHART":
			// CHART type.id name title units family context type priority ...
			require.GreaterOrEqual(t, len(fields), 8, line)
			current = charts[fields[1]]
			if current == nil {
				current = &wireChart{
					labels: make(map[string]string),
				}
				charts[fields[1]] = current
			}
			current.title, current.units, current.family = fields[3], fields[4], fields[5]
			current.context, current.chartType = fields[6], fields[7]
		case "DIMENSION":
			// DIMENSION id name algorithm multiplier divisor options
			require.NotNil(t, current, line)
			require.Len(t, fields, 7, line)
			multiplier, err := strconv.Atoi(fields[4])
			require.NoError(t, err)
			divisor, err := strconv.Atoi(fields[5])
			require.NoError(t, err)
			dim := wireDimension{
				name:       fields[2],
				algorithm:  fields[3],
				multiplier: multiplier,
				divisor:    divisor,
				hidden:     slices.Contains(strings.Fields(fields[6]), "hidden"),
			}
			if i := slices.IndexFunc(current.dimensions, func(d wireDimension) bool { return d.name == dim.name }); i >= 0 {
				current.dimensions[i] = dim
			} else {
				current.dimensions = append(current.dimensions, dim)
			}
		case "CLABEL":
			require.NotNil(t, current, line)
			require.GreaterOrEqual(t, len(fields), 3, line)
			if !strings.HasPrefix(fields[1], "_") {
				current.labels[fields[1]] = fields[2]
			}
		case "VARIABLE":
			// VARIABLE CHART name = value; a definition declares each one once.
			require.Len(t, fields, 5, line)
			if current != nil && !slices.Contains(current.variables, fields[2]) {
				current.variables = append(current.variables, fields[2])
			}
		case "BEGIN":
			// Updates follow; a VARIABLE line there carries a value.
			current = nil
		}
	}
	return charts
}

// publishedCharts runs two collections through the V2 job path, so the CPU
// charts have an interval, and returns every chart the job defined.
func publishedCharts(t *testing.T) []wireChart {
	t.Helper()
	var charts []wireChart
	synctest.Test(t, func(t *testing.T) {
		c := configuredCollector()
		c.Vnode = "fixture"
		c.newSession = func(context.Context) (querySession, error) {
			return &fakeSession{
				results: timedQueryResults(),
			}, nil
		}
		collect := startJob(t, c, vnodes.VirtualNode{
			Name:     "fixture",
			Hostname: "fixture",
			GUID:     "245dea48-ae2a-4697-a22e-050e7e358afb",
		})
		output := collect(c.UpdateEvery)
		time.Sleep(time.Duration(c.UpdateEvery) * time.Second)
		output += collect(2 * c.UpdateEvery)
		for _, chart := range parseCharts(t, output) {
			// The job runtime's own collection charts are not the collector's.
			if !strings.HasPrefix(chart.context, "netdata.") {
				charts = append(charts, *chart)
			}
		}
	})
	return charts
}

// TestCollector_PublishesSharedContexts compares the published charts with the
// other producers' definitions of the same contexts. The oracles of the native
// contexts are the native Windows Agent's
// src/collectors/windows.plugin/perflib-processor.c (system.cpu, cpu.cpu),
// src/collectors/common-contexts/system-ram.h (system.ram),
// src/collectors/windows.plugin/GetSystemRAM.c with
// src/collectors/common-contexts/mem-swap.h (mem.swap),
// src/collectors/windows.plugin/perflib-memory.c with
// src/collectors/common-contexts/mem-available.h and mem-pgfaults.h (the other
// mem.* charts; the swap I/O multiplier is the page size, 4096 on Windows),
// src/collectors/windows.plugin/perflib-pagefile.c (mem.pagefile_usage),
// src/collectors/windows.plugin/perflib-numa.c (mem.numa_node_mem_usage),
// src/collectors/windows.plugin/perflib-processes.c with
// src/collectors/common-contexts/system-processes.h (system.processes,
// system.threads, system.ctxt, system.processor_queue_length),
// src/collectors/windows.plugin/GetSystemUptime.c (system.uptime),
// src/collectors/timex.plugin/plugin_timex.c (system.clock_sync_offset, which
// the native agent does not chart; timex sends microseconds with a divisor of
// 1000, as WMI sends the Windows Time service's computed offset, or
// nanoseconds with a divisor of 1000000 when the kernel reports nanoseconds:
// milliseconds either way),
// src/collectors/windows.plugin/perflib-network.c (system.net, the net.*
// interface charts and the tcppackets and udppackets charts),
// src/collectors/proc.plugin/proc_net_netstat.c (ip.tcpsock, ip.tcpopens,
// ip.tcphandshake, ip.tcperrors and the udperrors charts, the TCP and UDP
// counters the native agent does not chart; only the dimensions Windows counts,
// and titles of the charts for both IP versions without the Linux "IPv4", as
// native ip.tcppackets has) and
// src/collectors/windows.plugin/perflib-storage.c (disk.space) with
// src/collectors/common-contexts/system-io.h and disk-*.h (system.io and the
// physical disk charts; the native disk.split dimension is named discards) and
// src/collectors/windows.plugin/perflib-smb.c (the smb.server_shares_* charts;
// a counts dimension is incremental, the open files and tree connects absolute,
// as perflib_rrddim_add derives them from the counter types) and
// src/collectors/windows.plugin/perflib-terminal-services.c
// (terminal_services.sessions; both session counts are raw counts, so
// perflib_rrddim_add makes them absolute).
// Native average times are sent in microseconds with a divisor of 1000, as
// common_disk_await and common_disk_svctm do. The ping.*
// entries are the wire form of the go.d ping collector's definitions, which
// TestPingChartsMatchPingCollector compares with that collector's template.
// Native CPU dimensions use the percent-of-total-delta algorithm, which this
// collector computes itself, so they arrive as absolute percentages. Native
// net.* and disk.space charts take their family from the interface or drive
// name, which a chart template cannot, so their families are not compared.
func TestCollector_PublishesSharedContexts(t *testing.T) {
	modes := []wireDimension{
		{name: "irq", algorithm: "absolute", multiplier: 1, divisor: 1},
		{name: "user", algorithm: "absolute", multiplier: 1, divisor: 1},
		{name: "system", algorithm: "absolute", multiplier: 1, divisor: 1},
		{name: "dpc", algorithm: "absolute", multiplier: 1, divisor: 1},
		{name: "idle", algorithm: "absolute", multiplier: 1, divisor: 1, hidden: true},
	}
	bandwidth := []wireDimension{
		{name: "received", algorithm: "incremental", multiplier: 8, divisor: 1000},
		{name: "sent", algorithm: "incremental", multiplier: -8, divisor: 1000},
	}
	inboundOutbound := []wireDimension{
		{name: "inbound", algorithm: "incremental", multiplier: 1, divisor: 1},
		{name: "outbound", algorithm: "incremental", multiplier: -1, divisor: 1},
	}
	single := func(name, algorithm string, divisor int) []wireDimension {
		return []wireDimension{{name: name, algorithm: algorithm, multiplier: 1, divisor: divisor}}
	}
	// readsWrites lists the native disk charts' reads and negated writes.
	readsWrites := func(algorithm string, divisor int) []wireDimension {
		return []wireDimension{
			{name: "reads", algorithm: algorithm, multiplier: 1, divisor: divisor},
			{name: "writes", algorithm: algorithm, multiplier: -1, divisor: divisor},
		}
	}
	receivedSent := []wireDimension{
		{name: "received", algorithm: "incremental", multiplier: 1, divisor: 1},
		{name: "sent", algorithm: "incremental", multiplier: -1, divisor: 1},
	}
	// unscaled lists dimensions without a multiplier or divisor.
	unscaled := func(algorithm string, names ...string) []wireDimension {
		var dims []wireDimension
		for _, name := range names {
			dims = append(dims, wireDimension{
				name:       name,
				algorithm:  algorithm,
				multiplier: 1,
				divisor:    1,
			})
		}
		return dims
	}
	native := map[string]wireChart{
		"system.cpu": {
			family:     "cpu",
			title:      "Total CPU Utilization",
			units:      "percentage",
			chartType:  "stacked",
			dimensions: modes,
		},
		"cpu.cpu": {
			family:     "utilization",
			title:      "Core Utilization",
			units:      "percentage",
			chartType:  "stacked",
			dimensions: modes,
		},
		"system.ram": {
			family:    "ram",
			title:     "System RAM",
			units:     "MiB",
			chartType: "stacked",
			dimensions: []wireDimension{
				{name: "free", algorithm: "absolute", multiplier: 1, divisor: 1048576},
				{name: "used", algorithm: "absolute", multiplier: 1, divisor: 1048576},
			},
		},
		"mem.available": {
			family:     "overview",
			title:      "Available RAM for applications",
			units:      "MiB",
			chartType:  "area",
			dimensions: single("avail", "absolute", 1048576),
		},
		"mem.swap": {
			family:    "swap",
			title:     "System Swap",
			units:     "MiB",
			chartType: "stacked",
			dimensions: []wireDimension{
				{name: "free", algorithm: "absolute", multiplier: 1, divisor: 1048576},
				{name: "used", algorithm: "absolute", multiplier: 1, divisor: 1048576},
			},
		},
		"mem.swap_iops": {
			family:    "swap",
			title:     "Swap Operations",
			units:     "operations/s",
			chartType: "stacked",
			dimensions: []wireDimension{
				{name: "read", algorithm: "incremental", multiplier: 1, divisor: 1},
				{name: "write", algorithm: "incremental", multiplier: 1, divisor: -1},
			},
		},
		"mem.swapio": {
			family:    "swap",
			title:     "Swap I/O",
			units:     "KiB/s",
			chartType: "area",
			dimensions: []wireDimension{
				{name: "in", algorithm: "incremental", multiplier: 4096, divisor: 1024},
				{name: "out", algorithm: "incremental", multiplier: -4096, divisor: 1024},
			},
		},
		"mem.committed": {
			family:     "mem",
			title:      "Committed Memory",
			units:      "bytes",
			chartType:  "line",
			dimensions: unscaled("absolute", "committed", "limit"),
		},
		"mem.system_cache": {
			family:     "mem",
			title:      "System Cache Memory",
			units:      "bytes",
			chartType:  "line",
			dimensions: unscaled("absolute", "cache", "peak", "resident"),
		},
		"mem.page_lists": {
			family:     "mem",
			title:      "Memory Page Lists",
			units:      "bytes",
			chartType:  "stacked",
			dimensions: unscaled("absolute", "free", "modified", "standby_core", "standby_normal", "standby_reserve"),
		},
		"mem.system_pool_size": {
			family:     "mem",
			title:      "System Memory Pool",
			units:      "bytes",
			chartType:  "stacked",
			dimensions: unscaled("absolute", "paged", "non-paged"),
		},
		"mem.system_pool_paged": {
			family:     "mem",
			title:      "Paged System Memory Pool",
			units:      "bytes",
			chartType:  "line",
			dimensions: unscaled("absolute", "resident"),
		},
		"mem.system_pool_allocs": {
			family:     "mem",
			title:      "System Pool Allocations",
			units:      "allocations/s",
			chartType:  "line",
			dimensions: unscaled("incremental", "paged", "non-paged"),
		},
		"mem.system_page_table_entries": {
			family:     "mem",
			title:      "Unused page table entries.",
			units:      "pages",
			chartType:  "line",
			dimensions: unscaled("absolute", "free"),
		},
		"mem.system_code": {
			family:     "mem",
			title:      "System Code Memory",
			units:      "bytes",
			chartType:  "line",
			dimensions: unscaled("absolute", "resident", "total"),
		},
		"mem.system_drivers": {
			family:     "mem",
			title:      "System Driver Memory",
			units:      "bytes",
			chartType:  "line",
			dimensions: unscaled("absolute", "resident", "total"),
		},
		"mem.pgfaults": {
			family:    "page faults",
			title:     "Memory Page Faults",
			units:     "faults/s",
			chartType: "line",
			dimensions: []wireDimension{
				{name: "minor", algorithm: "incremental", multiplier: 1, divisor: 1},
				{name: "major", algorithm: "incremental", multiplier: -1, divisor: 1},
			},
		},
		"mem.page_faults_breakdown": {
			family:     "page faults",
			title:      "Memory Page Fault Breakdown",
			units:      "faults/s",
			chartType:  "line",
			dimensions: unscaled("incremental", "cache", "demand_zero", "transition", "write_copies"),
		},
		"mem.transition_repurposed": {
			family:     "page faults",
			title:      "Transition Pages Repurposed",
			units:      "pages/s",
			chartType:  "line",
			dimensions: unscaled("incremental", "repurposed"),
		},
		"mem.pagefile_usage": {
			family:     "pagefile",
			title:      "Paging file usage",
			units:      "bytes",
			chartType:  "stacked",
			dimensions: unscaled("absolute", "free", "used"),
		},
		"mem.numa_node_mem_usage": {
			family:    "numa",
			title:     "NUMA Node Memory Usage",
			units:     "bytes",
			chartType: "line",
			dimensions: []wireDimension{
				{name: "free", algorithm: "absolute", multiplier: 1048576, divisor: 1},
				{name: "standby", algorithm: "absolute", multiplier: 1048576, divisor: 1},
			},
		},
		"system.processes": {
			family:     "processes",
			title:      "System Processes",
			units:      "processes",
			chartType:  "line",
			dimensions: single("running", "absolute", 1),
		},
		"system.threads": {
			family:     "processes",
			title:      "Threads",
			units:      "threads",
			chartType:  "line",
			dimensions: single("threads", "absolute", 1),
		},
		"system.ctxt": {
			family:     "processes",
			title:      "CPU Context Switches",
			units:      "context switches/s",
			chartType:  "line",
			dimensions: single("switches", "incremental", 1),
		},
		"system.processor_queue_length": {
			family:     "system",
			title:      "The number of threads in the processor queue.",
			units:      "threads",
			chartType:  "line",
			dimensions: single("threads", "absolute", 1),
		},
		"system.uptime": {
			family:     "uptime",
			title:      "System Uptime",
			units:      "seconds",
			chartType:  "line",
			dimensions: single("uptime", "absolute", 1000),
		},
		"system.clock_sync_offset": {
			family:     "clock synchronization",
			title:      "Computed Time Offset Between Local System and Reference Clock",
			units:      "milliseconds",
			chartType:  "line",
			dimensions: single("offset", "absolute", 1000),
		},
		"system.net": {
			family:     "network",
			title:      "Physical Network Interfaces Aggregated Bandwidth",
			units:      "kilobits/s",
			chartType:  "area",
			dimensions: bandwidth,
		},
		"net.net": {
			title:      "Bandwidth",
			units:      "kilobits/s",
			chartType:  "area",
			dimensions: bandwidth,
			variables:  []string{"nic_speed_max"},
		},
		"net.packets": {
			title:     "Packets",
			units:     "packets/s",
			chartType: "line",
			dimensions: []wireDimension{
				{name: "received", algorithm: "incremental", multiplier: 1, divisor: 1},
				{name: "sent", algorithm: "incremental", multiplier: -1, divisor: 1},
			},
		},
		"net.speed": {
			title:      "Interface Speed",
			units:      "kilobits/s",
			chartType:  "line",
			dimensions: single("speed", "absolute", 1000),
		},
		"net.errors": {
			title:      "Interface Errors",
			units:      "errors/s",
			chartType:  "line",
			dimensions: inboundOutbound,
		},
		"net.drops": {
			title:      "Interface Drops",
			units:      "drops/s",
			chartType:  "line",
			dimensions: inboundOutbound,
		},
		"net.unknown_protocol_packets": {
			title:      "Packets Received with Unknown Protocol",
			units:      "packets/s",
			chartType:  "line",
			dimensions: single("inbound", "incremental", 1),
		},
		"net.queue_length": {
			title:      "Interface Output Queue Length",
			units:      "packets",
			chartType:  "line",
			dimensions: single("length", "absolute", 1),
		},
		"net.rsc_connections": {
			title:      "Active TCP Connections Offloaded by RSC",
			units:      "connections",
			chartType:  "line",
			dimensions: single("connections", "absolute", 1),
		},
		"net.rsc_packets": {
			title:      "TCP RSC Coalesced Packets",
			units:      "packets/s",
			chartType:  "line",
			dimensions: single("packets", "incremental", 1),
		},
		"net.rsc_exceptions": {
			title:      "TCP RSC Exceptions",
			units:      "exceptions/s",
			chartType:  "line",
			dimensions: single("exceptions", "incremental", 1),
		},
		"net.rsc_average_packet_size": {
			title:      "TCP RSC Average Packet Size",
			units:      "bytes",
			chartType:  "line",
			dimensions: single("average", "absolute", 1),
		},
		"net.chimney_connections": {
			title:      "Active TCP Connections Offloaded with Chimney",
			units:      "connections",
			chartType:  "line",
			dimensions: single("connections", "absolute", 1),
		},
		"disk.space": {
			title:     "Disk Space Usage",
			units:     "GiB",
			chartType: "stacked",
			dimensions: []wireDimension{
				{name: "avail", algorithm: "absolute", multiplier: 1, divisor: 1073741824},
				{name: "used", algorithm: "absolute", multiplier: 1, divisor: 1073741824},
			},
		},
		"system.io": {
			family:    "disk",
			title:     "Disk I/O",
			units:     "KiB/s",
			chartType: "area",
			dimensions: []wireDimension{
				{name: "reads", algorithm: "incremental", multiplier: 1, divisor: 1024},
				{name: "writes", algorithm: "incremental", multiplier: -1, divisor: 1024},
			},
		},
		"disk.io": {
			family:    "io",
			title:     "Disk I/O Bandwidth",
			units:     "KiB/s",
			chartType: "area",
			dimensions: []wireDimension{
				{name: "reads", algorithm: "incremental", multiplier: 1, divisor: 1024},
				{name: "writes", algorithm: "incremental", multiplier: -1, divisor: 1024},
			},
		},
		"disk.ops": {
			family:     "ops",
			title:      "Disk Completed I/O Operations",
			units:      "operations/s",
			chartType:  "line",
			dimensions: readsWrites("incremental", 1),
		},
		"disk.util": {
			family:     "utilization",
			title:      "Disk Utilization Time",
			units:      "% of time working",
			chartType:  "area",
			dimensions: single("utilization", "absolute", 1),
		},
		"disk.busy": {
			family:     "utilization",
			title:      "Disk Busy Time",
			units:      "milliseconds",
			chartType:  "area",
			dimensions: single("busy", "incremental", 1),
		},
		"disk.iotime": {
			family:     "utilization",
			title:      "Disk Total I/O Time",
			units:      "milliseconds/s",
			chartType:  "area",
			dimensions: readsWrites("incremental", 1),
		},
		"disk.qops": {
			family:     "ops",
			title:      "Disk Current I/O Operations",
			units:      "operations",
			chartType:  "line",
			dimensions: single("operations", "absolute", 1),
		},
		"disk.await": {
			family:     "latency",
			title:      "Average Completed I/O Operation Time",
			units:      "milliseconds/operation",
			chartType:  "line",
			dimensions: readsWrites("absolute", 1000),
		},
		"disk.svctm": {
			family:     "latency",
			title:      "Average Service Time",
			units:      "milliseconds/operation",
			chartType:  "line",
			dimensions: single("svctm", "absolute", 1000),
		},
		"disk.avgsz": {
			family:     "io",
			title:      "Average Completed I/O Operation Bandwidth",
			units:      "KiB/operation",
			chartType:  "area",
			dimensions: readsWrites("absolute", 1024),
		},
		"disk.split": {
			family:     "iops",
			title:      "Split I/O Operations",
			units:      "operations/s",
			chartType:  "line",
			dimensions: single("discards", "incremental", 1),
		},
		"ip.tcppackets": {
			family:     "tcp",
			title:      "TCP Packets",
			units:      "packets/s",
			chartType:  "area",
			dimensions: receivedSent,
		},
		"ipv4.tcppackets": {
			family:     "tcp",
			title:      "IPv4 TCP Packets",
			units:      "packets/s",
			chartType:  "area",
			dimensions: receivedSent,
		},
		"ipv6.tcppackets": {
			family:     "tcp6",
			title:      "IPv6 TCP Packets",
			units:      "packets/s",
			chartType:  "area",
			dimensions: receivedSent,
		},
		"ipv4.udppackets": {
			family:     "udp",
			title:      "IPv4 UDP Packets",
			units:      "packets/s",
			chartType:  "area",
			dimensions: receivedSent,
		},
		"ipv6.udppackets": {
			family:     "udp6",
			title:      "IPv6 UDP Packets",
			units:      "packets/s",
			chartType:  "area",
			dimensions: receivedSent,
		},
		"ip.tcpsock": {
			family:     "tcp",
			title:      "TCP Connections",
			units:      "active connections",
			chartType:  "line",
			dimensions: single("connections", "absolute", 1),
		},
		"ip.tcpopens": {
			family:     "tcp",
			title:      "TCP Opens",
			units:      "connections/s",
			chartType:  "line",
			dimensions: unscaled("incremental", "active", "passive"),
		},
		"ip.tcphandshake": {
			family:     "tcp",
			title:      "TCP Handshake Issues",
			units:      "events/s",
			chartType:  "line",
			dimensions: unscaled("incremental", "EstabResets", "AttemptFails"),
		},
		"ip.tcperrors": {
			family:    "tcp",
			title:     "TCP Errors",
			units:     "packets/s",
			chartType: "line",
			dimensions: []wireDimension{
				{name: "RetransSegs", algorithm: "incremental", multiplier: -1, divisor: 1},
			},
		},
		"ipv4.udperrors": {
			family:     "udp",
			title:      "IPv4 UDP Errors",
			units:      "events/s",
			chartType:  "line",
			dimensions: unscaled("incremental", "InErrors", "NoPorts"),
		},
		"ipv6.udperrors": {
			family:     "udp6",
			title:      "IPv6 UDP Errors",
			units:      "events/s",
			chartType:  "line",
			dimensions: unscaled("incremental", "InErrors", "NoPorts"),
		},
		"smb.server_shares_current_open_file_count": {
			family:     "shares",
			title:      "Current open file count on the SMB share",
			units:      "files",
			chartType:  "line",
			dimensions: single("open", "absolute", 1),
		},
		"smb.server_shares_write_requests": {
			family:     "shares",
			title:      "Write requests on the SMB share",
			units:      "requests/s",
			chartType:  "line",
			dimensions: single("writes", "incremental", 1),
		},
		"smb.server_shares_read_requests": {
			family:     "shares",
			title:      "Read requests on the SMB share",
			units:      "requests/s",
			chartType:  "line",
			dimensions: single("reads", "incremental", 1),
		},
		"smb.server_shares_metadata_requests": {
			family:     "shares",
			title:      "Metadata requests on the SMB share",
			units:      "requests/s",
			chartType:  "line",
			dimensions: single("metadata", "incremental", 1),
		},
		"smb.server_shares_files_opened": {
			family:     "shares",
			title:      "Files opened on the SMB share",
			units:      "files/s",
			chartType:  "line",
			dimensions: single("opened", "incremental", 1),
		},
		"smb.server_shares_tree_connects": {
			family:     "shares",
			title:      "Tree connects on the SMB share",
			units:      "connections",
			chartType:  "line",
			dimensions: single("connects", "absolute", 1),
		},
		"smb.server_shares_traffic": {
			family:     "shares",
			title:      "Traffic on the SMB share",
			units:      "bytes/s",
			chartType:  "area",
			dimensions: receivedSent,
		},
		"terminal_services.sessions": {
			family:     "sessions",
			title:      "Terminal Services sessions",
			units:      "sessions",
			chartType:  "stacked",
			dimensions: unscaled("absolute", "active", "inactive"),
		},
		"ping.host_rtt": {
			family:    "latency",
			title:     "Ping round-trip time",
			units:     "milliseconds",
			chartType: "area",
			dimensions: []wireDimension{
				{name: "min", algorithm: "absolute", multiplier: 1, divisor: 1000},
				{name: "max", algorithm: "absolute", multiplier: 1, divisor: 1000},
				{name: "avg", algorithm: "absolute", multiplier: 1, divisor: 1000},
			},
		},
		"ping.host_std_dev_rtt": {
			family:    "latency",
			title:     "Ping round-trip time standard deviation",
			units:     "milliseconds",
			chartType: "line",
			dimensions: []wireDimension{
				{name: "std_dev", algorithm: "absolute", multiplier: 1, divisor: 1000},
			},
		},
		"ping.host_packet_loss": {
			family:    "packet loss",
			title:     "Ping packet loss",
			units:     "percentage",
			chartType: "line",
			dimensions: []wireDimension{
				{name: "loss", algorithm: "absolute", multiplier: 1, divisor: 1000},
			},
		},
	}
	instance := func(context string, labels map[string]string) wireChart {
		chart := native[context]
		chart.context = context
		chart.labels = labels
		return chart
	}

	// One chart per instance in the fixture. Ethernet and Ethernet#1 report
	// only their byte totals; the interface without traffic has none, and the
	// adapters that are also interfaces are charted once, as real. Label
	// values carry slashes for backslashes, as the Agent stores the native
	// pagefile label.
	want := []wireChart{
		instance("system.cpu", map[string]string{}),
		instance("cpu.cpu", map[string]string{"cpu": "cpu0"}),
		instance("cpu.cpu", map[string]string{"cpu": "cpu1"}),
		instance("system.ram", map[string]string{}),
		instance("mem.pagefile_usage", map[string]string{"pagefile": "C:/pagefile.sys"}),
		instance("mem.pagefile_usage", map[string]string{"pagefile": "D:/pagefile.sys"}),
		instance("mem.numa_node_mem_usage", map[string]string{"node": "0"}),
		instance("mem.numa_node_mem_usage", map[string]string{"node": "1"}),
		instance("system.processes", map[string]string{}),
		instance("system.threads", map[string]string{}),
		instance("system.ctxt", map[string]string{}),
		instance("system.processor_queue_length", map[string]string{}),
		instance("system.uptime", map[string]string{}),
		instance("system.clock_sync_offset", map[string]string{}),
		instance("terminal_services.sessions", map[string]string{}),
		instance("system.net", map[string]string{}),
		instance("net.net", map[string]string{"device": "Ethernet", "interface_type": "real"}),
		instance("net.net", map[string]string{"device": "Ethernet#1", "interface_type": "real"}),
		instance("disk.space", map[string]string{
			"mount_point":   "C:",
			"drive_type":    "fixed",
			"filesystem":    "NTFS",
			"rw_mode":       "rw",
			"serial_number": "0xBEF3E296",
		}),
		instance("disk.space", map[string]string{
			"mount_point":   "D:",
			"drive_type":    "fixed",
			"filesystem":    "unknown",
			"rw_mode":       "rw",
			"serial_number": "0xABCD",
		}),
		instance("system.io", map[string]string{}),
		instance("ping.host_rtt", map[string]string{"host": "windows.example.com"}),
		instance("ping.host_std_dev_rtt", map[string]string{"host": "windows.example.com"}),
		instance("ping.host_packet_loss", map[string]string{"host": "windows.example.com"}),
	}
	for context := range native {
		if strings.HasPrefix(context, "ip.") || strings.HasPrefix(context, "ipv4.") ||
			strings.HasPrefix(context, "ipv6.") {
			want = append(want, instance(context, map[string]string{}))
		}
		if strings.HasPrefix(context, "mem.") && context != "mem.pagefile_usage" &&
			context != "mem.numa_node_mem_usage" {
			want = append(want, instance(context, map[string]string{}))
		}
		if strings.HasPrefix(context, "net.") {
			want = append(want,
				instance(context, map[string]string{"device": "Uplink", "interface_type": "real"}),
				instance(context, map[string]string{"device": "Team Member", "interface_type": "virtual"}),
			)
		}
		// The native agent labels each share with its instance name too; the
		// plugin protocol sends its backslashes as slashes, and the Agent then
		// stores the share labels of both as //_/C_ and //_/IPC_.
		if strings.HasPrefix(context, "smb.") {
			want = append(want,
				instance(context, map[string]string{"share": "//*/C$"}),
				instance(context, map[string]string{"share": "//*/IPC$"}),
			)
		}
		// The native agent labels the device ID with slashes for backslashes
		// too, and leaves out the empty mount point of a disk without a
		// lettered volume (string_strdupz returns NULL for an empty string).
		if strings.HasPrefix(context, "disk.") && context != "disk.space" {
			want = append(want,
				instance(context, map[string]string{
					"device":      "Disk 0",
					"mount_point": "C:",
					"model":       "QEMU HARDDISK",
					"device_id":   "//./PHYSICALDRIVE0",
				}),
				instance(context, map[string]string{
					"device":    "Disk 1",
					"model":     "Virtual Disk",
					"device_id": "//./PHYSICALDRIVE1",
				}),
			)
		}
	}
	// WMI's own contexts have no other producer to compare with;
	// TestCollector_JobV2ClockOffset and
	// TestCollector_TimeServiceCountersReachCharts cover the clock ones.
	published := slices.DeleteFunc(publishedCharts(t), func(chart wireChart) bool {
		return strings.HasPrefix(chart.context, "wmi.")
	})
	for i, chart := range published {
		if strings.HasPrefix(chart.context, "net.") || chart.context == "disk.space" {
			published[i].family = ""
		}
	}
	assert.ElementsMatch(t, want, published)
}

// TestCollector_InterfaceCountersFeedNativeDimensions checks that each WMI
// counter reaches the native chart dimension that reads it.
func TestCollector_InterfaceCountersFeedNativeDimensions(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := configuredCollector()
		c.Vnode = "fixture"
		c.newSession = func(context.Context) (querySession, error) {
			return &fakeSession{
				results: timedQueryResults(),
			}, nil
		}
		collect := startJob(t, c, vnodes.VirtualNode{
			Name:     "fixture",
			Hostname: "fixture",
			GUID:     "245dea48-ae2a-4697-a22e-050e7e358afb",
		})
		output := collect(c.UpdateEvery)
		charts := parseCharts(t, output)
		samples := parseSamples(t, output)

		want := make(map[string]map[nativeDimension]float64)
		got := make(map[string]map[nativeDimension]float64)
		for device, base := range map[string]uint64{"Uplink": 1000, "Team Member": 2000} {
			want[device] = make(map[nativeDimension]float64)
			row := nativeInterfaceRow(device, base)
			for property, value := range row {
				if property != "Name" {
					want[device][nativeInterfaceCounters[property]] = float64(value.(uint64))
				}
			}
			// The native agent sets net.net's nic_speed_max to the speed in kilobits/s.
			want[device][nativeDimension{"net.net", "$nic_speed_max"}] = float64(
				row["CurrentBandwidth"].(uint64),
			) / 1000
			got[device] = make(map[nativeDimension]float64)
			for id, chart := range charts {
				if chart.labels["device"] != device {
					continue
				}
				for dimension, value := range samples[id] {
					got[device][nativeDimension{chart.context, dimension}] = value
				}
			}
		}
		assert.Equal(t, want, got)
	})
}

// TestCollector_ProtocolCountersFeedNativeDimensions checks that each TCP and
// UDP counter reaches the dimension that reads the same counter: the native
// Windows Agent's (src/collectors/windows.plugin/perflib-network.c) for the
// segments and datagrams per IP version, and for the segments of both versions
// summed (tcp46), and the Linux Agent's
// (src/collectors/proc.plugin/proc_net_netstat.c) for the MIB-II counters the
// native agent does not chart: TCP for both versions summed, as Linux counts
// TCP, and UDP per version.
func TestCollector_ProtocolCountersFeedNativeDimensions(t *testing.T) {
	tcp := map[string]nativeDimension{
		"SegmentsReceivedPersec":      {"ip.tcppackets", "received"},
		"SegmentsSentPersec":          {"ip.tcppackets", "sent"},
		"ConnectionsEstablished":      {"ip.tcpsock", "connections"},
		"ConnectionsActive":           {"ip.tcpopens", "active"},
		"ConnectionsPassive":          {"ip.tcpopens", "passive"},
		"ConnectionsReset":            {"ip.tcphandshake", "EstabResets"},
		"ConnectionFailures":          {"ip.tcphandshake", "AttemptFails"},
		"SegmentsRetransmittedPersec": {"ip.tcperrors", "RetransSegs"},
	}
	perVersion := map[string]map[string]nativeDimension{
		queryTCPv4: {
			"SegmentsReceivedPersec": {"ipv4.tcppackets", "received"},
			"SegmentsSentPersec":     {"ipv4.tcppackets", "sent"},
		},
		queryTCPv6: {
			"SegmentsReceivedPersec": {"ipv6.tcppackets", "received"},
			"SegmentsSentPersec":     {"ipv6.tcppackets", "sent"},
		},
		queryUDPv4: {
			"DatagramsReceivedPersec": {"ipv4.udppackets", "received"},
			"DatagramsSentPersec":     {"ipv4.udppackets", "sent"},
			"DatagramsReceivedErrors": {"ipv4.udperrors", "InErrors"},
			"DatagramsNoPortPersec":   {"ipv4.udperrors", "NoPorts"},
		},
		queryUDPv6: {
			"DatagramsReceivedPersec": {"ipv6.udppackets", "received"},
			"DatagramsSentPersec":     {"ipv6.udppackets", "sent"},
			"DatagramsReceivedErrors": {"ipv6.udperrors", "InErrors"},
			"DatagramsNoPortPersec":   {"ipv6.udperrors", "NoPorts"},
		},
	}
	// Each counter of each version has its own value, so a counter or version
	// that reaches the wrong dimension changes the result.
	results := validQueryResults()
	rows := make(map[string]map[string]any)
	next := uint32(1000)
	for query, dimensions := range perVersion {
		row := make(map[string]any)
		properties := slices.Collect(maps.Keys(dimensions))
		if query == queryTCPv4 || query == queryTCPv6 {
			properties = slices.Collect(maps.Keys(tcp))
		}
		slices.Sort(properties)
		for _, property := range properties {
			row[property] = next
			next += 1000
		}
		rows[query] = row
		results[query] = fixtureRows(row)
	}
	want := make(map[nativeDimension]float64)
	for query, dimensions := range perVersion {
		for property, dimension := range dimensions {
			want[dimension] = float64(rows[query][property].(uint32))
		}
	}
	for property, dimension := range tcp {
		want[dimension] = float64(rows[queryTCPv4][property].(uint32) + rows[queryTCPv6][property].(uint32))
	}

	synctest.Test(t, func(t *testing.T) {
		c := configuredCollector()
		c.Vnode = "fixture"
		c.newSession = func(context.Context) (querySession, error) {
			return &fakeSession{
				results: results,
			}, nil
		}
		collect := startJob(t, c, vnodes.VirtualNode{
			Name:     "fixture",
			Hostname: "fixture",
			GUID:     "245dea48-ae2a-4697-a22e-050e7e358afb",
		})
		output := collect(c.UpdateEvery)
		samples := parseSamples(t, output)

		got := make(map[nativeDimension]float64)
		for id, chart := range parseCharts(t, output) {
			if !strings.HasPrefix(chart.context, "ip.") && !strings.HasPrefix(chart.context, "ipv4.") &&
				!strings.HasPrefix(chart.context, "ipv6.") {
				continue
			}
			for dimension, value := range samples[id] {
				got[nativeDimension{chart.context, dimension}] = value
			}
		}
		assert.Equal(t, want, got)
	})
}

// TestCollector_DiskCountersFeedNativeDimensions checks that each PhysicalDisk
// counter reaches the native chart dimension that reads it
// (src/collectors/windows.plugin/perflib-storage.c, do_physical_disk), and that
// the interval values follow the native formulas for the rates of
// timedDiskRow, whose read time wraps its 32 bits more than once per interval.
func TestCollector_DiskCountersFeedNativeDimensions(t *testing.T) {
	native := map[string]nativeDimension{
		"DiskReadBytesPersec":    {"disk.io", "reads"},
		"DiskWriteBytesPersec":   {"disk.io", "writes"},
		"DiskReadsPersec":        {"disk.ops", "reads"},
		"DiskWritesPersec":       {"disk.ops", "writes"},
		"SplitIOPerSec":          {"disk.split", "discards"},
		"CurrentDiskQueueLength": {"disk.qops", "operations"},
	}
	// Native times in whole milliseconds: 100 ns units / 10000.
	nativeTimes := map[string]nativeDimension{
		"PercentDiskTime":      {"disk.busy", "busy"},
		"PercentDiskReadTime":  {"disk.iotime", "reads"},
		"PercentDiskWriteTime": {"disk.iotime", "writes"},
	}
	synctest.Test(t, func(t *testing.T) {
		c := configuredCollector()
		c.Vnode = "fixture"
		c.newSession = func(context.Context) (querySession, error) {
			return &fakeSession{
				results: timedQueryResults(),
			}, nil
		}
		collect := startJob(t, c, vnodes.VirtualNode{
			Name:     "fixture",
			Hostname: "fixture",
			GUID:     "245dea48-ae2a-4697-a22e-050e7e358afb",
		})
		charts := parseCharts(t, collect(c.UpdateEvery))
		time.Sleep(time.Duration(c.UpdateEvery) * time.Second)
		output := collect(2 * c.UpdateEvery)
		samples := parseSamples(t, output)
		maps.Copy(charts, parseCharts(t, output))

		want := make(map[string]map[nativeDimension]float64)
		var totalRead, totalWritten float64
		for device, scale := range map[string]uint64{"Disk 0": 1, "Disk 1": 2} {
			row := timedDiskRow("", scale)
			want[device] = make(map[nativeDimension]float64)
			for property, dimension := range native {
				switch value := row[property].(type) {
				case uint32:
					want[device][dimension] = float64(value)
				case uint64:
					want[device][dimension] = float64(value)
				}
			}
			for property, dimension := range nativeTimes {
				want[device][dimension] = float64(row[property].(uint64) / 10000)
			}
			totalRead += float64(row["DiskReadBytesPersec"].(uint64))
			totalWritten += float64(row["DiskWriteBytesPersec"].(uint64))
		}
		// Utilization: 100 less the idle share. Times: reads take scale ms,
		// writes 4 * scale ms, and a transfer (20000 * scale + 2000 * 4 * scale) /
		// 22000 ms on average, all sent in whole microseconds. Sizes in bytes.
		for device, values := range map[string]map[nativeDimension]float64{
			"Disk 0": {
				{"disk.util", "utilization"}: 40,
				{"disk.await", "reads"}:      1000,
				{"disk.await", "writes"}:     4000,
				{"disk.svctm", "svctm"}:      1272,
				{"disk.avgsz", "reads"}:      4096,
				{"disk.avgsz", "writes"}:     65536,
			},
			"Disk 1": {
				{"disk.util", "utilization"}: 80,
				{"disk.await", "reads"}:      2000,
				{"disk.await", "writes"}:     8000,
				{"disk.svctm", "svctm"}:      2545,
				{"disk.avgsz", "reads"}:      8192,
				{"disk.avgsz", "writes"}:     131072,
			},
		} {
			maps.Copy(want[device], values)
		}
		want[""] = map[nativeDimension]float64{
			{"system.io", "reads"}:  totalRead,
			{"system.io", "writes"}: totalWritten,
		}

		got := make(map[string]map[nativeDimension]float64)
		for id, chart := range charts {
			if !strings.HasPrefix(chart.context, "disk.") && chart.context != "system.io" ||
				chart.context == "disk.space" {
				continue
			}
			device := chart.labels["device"]
			if got[device] == nil {
				got[device] = make(map[nativeDimension]float64)
			}
			for dimension, value := range samples[id] {
				got[device][nativeDimension{chart.context, dimension}] = value
			}
		}
		assert.Equal(t, want, got)
	})
}

// TestCollector_SystemCountersFeedNativeDimensions checks that each counter of
// the system group reaches the native chart dimension that reads it
// (src/collectors/windows.plugin/perflib-processes.c, do_processes and
// do_processor_queue, and perflib-terminal-services.c), and that uptime arrives
// in milliseconds since boot, as GetSystemUptime.c sends GetTickCount64().
func TestCollector_SystemCountersFeedNativeDimensions(t *testing.T) {
	native := map[string]nativeDimension{
		"Processes":             {"system.processes", "running"},
		"Threads":               {"system.threads", "threads"},
		"ContextSwitchesPersec": {"system.ctxt", "switches"},
		"ProcessorQueueLength":  {"system.processor_queue_length", "threads"},
	}
	row := systemRow()
	boot, now, frequency := row["SystemUpTime"].(uint64), row["Timestamp_Object"].(uint64), row["Frequency_Object"].(uint64)
	want := map[nativeDimension]float64{
		{"system.uptime", "uptime"}: float64((now - boot) * 1000 / frequency),
	}
	for property, dimension := range native {
		want[dimension] = float64(row[property].(uint32))
	}
	// The system group also reads the Terminal Services counter set, whose
	// counters src/collectors/windows.plugin/perflib-terminal-services.c charts.
	sessions := terminalServicesRow()
	for property, dimension := range map[string]nativeDimension{
		"ActiveSessions":   {"terminal_services.sessions", "active"},
		"InactiveSessions": {"terminal_services.sessions", "inactive"},
	} {
		want[dimension] = float64(sessions[property].(uint32))
	}

	synctest.Test(t, func(t *testing.T) {
		c := configuredCollector()
		c.Vnode = "fixture"
		c.newSession = func(context.Context) (querySession, error) {
			return &fakeSession{
				results: timedQueryResults(),
			}, nil
		}
		collect := startJob(t, c, vnodes.VirtualNode{
			Name:     "fixture",
			Hostname: "fixture",
			GUID:     "245dea48-ae2a-4697-a22e-050e7e358afb",
		})
		output := collect(c.UpdateEvery)
		samples := parseSamples(t, output)

		got := make(map[nativeDimension]float64)
		for id, chart := range parseCharts(t, output) {
			for dimension, value := range samples[id] {
				if _, ok := want[nativeDimension{chart.context, dimension}]; ok {
					got[nativeDimension{chart.context, dimension}] = value
				}
			}
		}
		assert.Equal(t, want, got)
	})
}

// querySessionFunc answers each query with a function, which can act on the
// poller's clock while the query runs.
type querySessionFunc func(wql string) ([]map[string]any, error)

func (f querySessionFunc) Query(_ context.Context, wql string) ([]map[string]any, error) {
	return f(wql)
}

func (querySessionFunc) Close() {}

// TestCollector_JobV2ClockOffset checks the host's clock offset through the V2
// job path. The host reads its clock in the middle of each System query, so the
// chart shows exactly how far the host's clock is ahead of the poller's, in
// milliseconds. A query that took longer than a second leaves a gap; a run of
// such queries longer than the default chart expiry and the metric store's
// retention keeps the chart, which shows the offset again with the next fast
// query.
func TestCollector_JobV2ClockOffset(t *testing.T) {
	// fileTime is a FILETIME: 100 ns ticks since 1601-01-01 UTC, 11644473600
	// seconds before the Unix epoch.
	fileTime := func(clock time.Time) uint64 {
		return uint64(clock.UnixNano()/100) + 11644473600*10000000
	}
	type collection struct {
		queryTime time.Duration
		hostAhead time.Duration
		// wantMilliseconds is the charted offset, when the collection has one.
		wantMilliseconds []float64
	}
	collections := []collection{
		{queryTime: 300 * time.Millisecond, hostAhead: 2500 * time.Millisecond, wantMilliseconds: []float64{2500}},
		{queryTime: time.Second, hostAhead: -750 * time.Millisecond, wantMilliseconds: []float64{-750}},
	}
	for range 21 {
		collections = append(collections, collection{queryTime: time.Second + time.Microsecond, hostAhead: time.Minute})
	}
	collections = append(collections, collection{hostAhead: 1234567 * time.Microsecond, wantMilliseconds: []float64{1234.567}})

	synctest.Test(t, func(t *testing.T) {
		c := configuredCollector()
		c.Vnode = "fixture"
		poller := time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)
		c.now = func() time.Time { return poller }
		var queryTime, hostAhead time.Duration
		results := timedQueryResults()
		c.newSession = func(context.Context) (querySession, error) {
			return querySessionFunc(func(wql string) ([]map[string]any, error) {
				if wql != querySystem {
					return results[wql].rows, results[wql].err
				}
				row := systemRow()
				row["Timestamp_Object"] = fileTime(poller.Add(queryTime/2 + hostAhead))
				poller = poller.Add(queryTime)
				return fixtureRows(row).rows, nil
			}), nil
		}
		collect := startJob(t, c, vnodes.VirtualNode{
			Name:     "fixture",
			Hostname: "fixture",
			GUID:     "245dea48-ae2a-4697-a22e-050e7e358afb",
		})

		var chartID string
		var dimension wireDimension
		for i, tc := range collections {
			queryTime, hostAhead = tc.queryTime, tc.hostAhead
			output := collect((i + 1) * c.UpdateEvery)
			for id, chart := range parseCharts(t, output) {
				// An obsolete chart is sent again without its dimension.
				if chart.context == "wmi.clock_offset" && len(chart.dimensions) == 1 {
					chartID, dimension = id, chart.dimensions[0]
				}
			}
			require.NotEmpty(t, chartID, "the clock offset chart is defined with the first collection")
			for line := range strings.Lines(output) {
				obsolete := strings.HasPrefix(line, "CHART '"+chartID+"'") && strings.Contains(line, "obsolete")
				assert.False(t, obsolete, "collection %d: %s", i+1, line)
			}
			var gotMilliseconds []float64
			if value, ok := parseSamples(t, output)[chartID][dimension.name]; ok {
				shown := value * float64(dimension.multiplier) / float64(dimension.divisor)
				gotMilliseconds = append(gotMilliseconds, shown)
			}
			assert.Equal(t, tc.wantMilliseconds, gotMilliseconds, "collection %d", i+1)
			time.Sleep(time.Duration(c.UpdateEvery) * time.Second)
		}
	})
}

// TestCollector_MemoryCountersFeedNativeDimensions checks that each WMI value
// reaches the native chart dimension that reads it: the Memory counters as
// src/collectors/windows.plugin/perflib-memory.c reads them (minor page faults
// are all faults less Pages/sec), the commit limit and available commit as
// GetSystemRAM.c charts the swap, each paging file's pages, in 4 KiB pages,
// as perflib-pagefile.c charts them, and each NUMA node's MiB as
// perflib-numa.c sends them.
func TestCollector_MemoryCountersFeedNativeDimensions(t *testing.T) {
	native := map[string]nativeDimension{
		"AvailableBytes":                  {"mem.available", "avail"},
		"PageReadsPersec":                 {"mem.swap_iops", "read"},
		"PageWritesPersec":                {"mem.swap_iops", "write"},
		"PagesInputPersec":                {"mem.swapio", "in"},
		"PagesOutputPersec":               {"mem.swapio", "out"},
		"CommittedBytes":                  {"mem.committed", "committed"},
		"CommitLimit":                     {"mem.committed", "limit"},
		"CacheBytes":                      {"mem.system_cache", "cache"},
		"CacheBytesPeak":                  {"mem.system_cache", "peak"},
		"SystemCacheResidentBytes":        {"mem.system_cache", "resident"},
		"FreeAndZeroPageListBytes":        {"mem.page_lists", "free"},
		"ModifiedPageListBytes":           {"mem.page_lists", "modified"},
		"StandbyCacheCoreBytes":           {"mem.page_lists", "standby_core"},
		"StandbyCacheNormalPriorityBytes": {"mem.page_lists", "standby_normal"},
		"StandbyCacheReserveBytes":        {"mem.page_lists", "standby_reserve"},
		"PoolPagedBytes":                  {"mem.system_pool_size", "paged"},
		"PoolNonpagedBytes":               {"mem.system_pool_size", "non-paged"},
		"PoolPagedResidentBytes":          {"mem.system_pool_paged", "resident"},
		"PoolPagedAllocs":                 {"mem.system_pool_allocs", "paged"},
		"PoolNonpagedAllocs":              {"mem.system_pool_allocs", "non-paged"},
		"FreeSystemPageTableEntries":      {"mem.system_page_table_entries", "free"},
		"SystemCodeResidentBytes":         {"mem.system_code", "resident"},
		"SystemCodeTotalBytes":            {"mem.system_code", "total"},
		"SystemDriverResidentBytes":       {"mem.system_drivers", "resident"},
		"SystemDriverTotalBytes":          {"mem.system_drivers", "total"},
		"CacheFaultsPersec":               {"mem.page_faults_breakdown", "cache"},
		"DemandZeroFaultsPersec":          {"mem.page_faults_breakdown", "demand_zero"},
		"TransitionFaultsPersec":          {"mem.page_faults_breakdown", "transition"},
		"WriteCopiesPersec":               {"mem.page_faults_breakdown", "write_copies"},
		"TransitionPagesRePurposedPersec": {"mem.transition_repurposed", "repurposed"},
	}
	number := func(value any) float64 {
		switch v := value.(type) {
		case uint32:
			return float64(v)
		case uint64:
			return float64(v)
		case string:
			n, err := strconv.ParseUint(v, 10, 64)
			require.NoError(t, err)
			return float64(n)
		}
		t.Fatalf("unexpected fixture value %v", value)
		return 0
	}

	// Values by the native pagefile or node label, empty for the host's charts.
	want := map[string]map[nativeDimension]float64{"": {}}
	counters := memoryCountersRow()
	for property, dimension := range native {
		want[""][dimension] = number(counters[property])
	}
	faults, pages := number(counters["PageFaultsPersec"]), number(counters["PagesPersec"])
	want[""][nativeDimension{"mem.pgfaults", "minor"}] = faults - pages
	want[""][nativeDimension{"mem.pgfaults", "major"}] = pages
	results := validQueryResults()
	osMemory := results[queryMemory].rows[0]
	commitLimit, commitAvailable := number(osMemory["totalvirtualmemorysize"]), number(osMemory["freevirtualmemory"])
	want[""][nativeDimension{"mem.swap", "free"}] = commitAvailable * 1024
	want[""][nativeDimension{"mem.swap", "used"}] = (commitLimit - commitAvailable) * 1024
	for _, row := range results[queryPagingFiles].rows {
		name := row["name"].(string)
		if name == "_Total" {
			continue
		}
		path := strings.ReplaceAll(strings.TrimPrefix(name, `\??\`), `\`, "/")
		used, size := number(row["percentusage"]), number(row["percentusage_base"])
		want[path] = map[nativeDimension]float64{
			{"mem.pagefile_usage", "used"}: used * 4096,
			{"mem.pagefile_usage", "free"}: (size - used) * 4096,
		}
	}

	for _, row := range results[queryNUMANodes].rows {
		node := row["name"].(string)
		if node == "_Total" {
			continue
		}
		want[node] = map[nativeDimension]float64{
			{"mem.numa_node_mem_usage", "free"}:    number(row["freeandzeropagelistmbytes"]),
			{"mem.numa_node_mem_usage", "standby"}: number(row["standbylistmbytes"]),
		}
	}

	synctest.Test(t, func(t *testing.T) {
		c := configuredCollector()
		c.Vnode = "fixture"
		c.newSession = func(context.Context) (querySession, error) {
			return &fakeSession{
				results: timedQueryResults(),
			}, nil
		}
		collect := startJob(t, c, vnodes.VirtualNode{
			Name:     "fixture",
			Hostname: "fixture",
			GUID:     "245dea48-ae2a-4697-a22e-050e7e358afb",
		})
		output := collect(c.UpdateEvery)
		samples := parseSamples(t, output)

		got := make(map[string]map[nativeDimension]float64)
		for id, chart := range parseCharts(t, output) {
			if !strings.HasPrefix(chart.context, "mem.") {
				continue
			}
			instance := chart.labels["pagefile"] + chart.labels["node"]
			if got[instance] == nil {
				got[instance] = make(map[nativeDimension]float64)
			}
			for dimension, value := range samples[id] {
				got[instance][nativeDimension{chart.context, dimension}] = value
			}
		}
		assert.Equal(t, want, got)
	})
}

// TestCollector_SMBShareCountersFeedNativeDimensions checks that each SMB
// Server Shares counter reaches the native chart dimension that reads it, per
// share (src/collectors/windows.plugin/perflib-smb.c, smb_share_charts and
// smb_share_traffic_chart).
func TestCollector_SMBShareCountersFeedNativeDimensions(t *testing.T) {
	native := map[string]nativeDimension{
		"CurrentOpenFileCount":   {"smb.server_shares_current_open_file_count", "open"},
		"WriteRequestsPersec":    {"smb.server_shares_write_requests", "writes"},
		"ReadRequestsPersec":     {"smb.server_shares_read_requests", "reads"},
		"MetadataRequestsPersec": {"smb.server_shares_metadata_requests", "metadata"},
		"FilesOpenedPersec":      {"smb.server_shares_files_opened", "opened"},
		"TreeConnectCount":       {"smb.server_shares_tree_connects", "connects"},
		"ReceivedBytesPersec":    {"smb.server_shares_traffic", "received"},
		"SentBytesPersec":        {"smb.server_shares_traffic", "sent"},
	}
	want := make(map[string]map[nativeDimension]float64)
	for _, row := range validQueryResults()[querySMBShares].rows {
		name := row["name"].(string)
		if name == "_Total" {
			continue
		}
		// The plugin protocol sends backslashes in label values as slashes.
		share := strings.ReplaceAll(name, `\`, "/")
		want[share] = make(map[nativeDimension]float64)
		for property, dimension := range native {
			want[share][dimension] = fixtureNumber(t, row[strings.ToLower(property)])
		}
	}

	synctest.Test(t, func(t *testing.T) {
		c := configuredCollector()
		c.Vnode = "fixture"
		c.newSession = func(context.Context) (querySession, error) {
			return &fakeSession{
				results: timedQueryResults(),
			}, nil
		}
		collect := startJob(t, c, vnodes.VirtualNode{
			Name:     "fixture",
			Hostname: "fixture",
			GUID:     "245dea48-ae2a-4697-a22e-050e7e358afb",
		})
		output := collect(c.UpdateEvery)
		samples := parseSamples(t, output)

		got := make(map[string]map[nativeDimension]float64)
		for id, chart := range parseCharts(t, output) {
			if !strings.HasPrefix(chart.context, "smb.") {
				continue
			}
			share := chart.labels["share"]
			if got[share] == nil {
				got[share] = make(map[nativeDimension]float64)
			}
			for dimension, value := range samples[id] {
				got[share][nativeDimension{chart.context, dimension}] = value
			}
		}
		assert.Equal(t, want, got)
	})
}

// TestCollector_SMBServerCountersReachCharts checks that each Server counter
// reaches exactly one SMB server chart dimension, unscaled: as a rate for the
// counts since the Server service started, and as is for the two point values,
// which Microsoft describes as the number of sessions currently active and of
// files currently open (Win32_PerfRawData_PerfNet_Server).
func TestCollector_SMBServerCountersReachCharts(t *testing.T) {
	pointValues := map[string]bool{
		"ServerSessions": true,
		"FilesOpen":      true,
	}
	// Every value of smbServerRow is distinct, so a value names its counter.
	row := smbServerRow()
	properties := make(map[float64]string, len(row))
	for property, value := range row {
		properties[fixtureNumber(t, value)] = property
	}

	synctest.Test(t, func(t *testing.T) {
		c := configuredCollector()
		c.Vnode = "fixture"
		c.newSession = func(context.Context) (querySession, error) {
			return &fakeSession{
				results: timedQueryResults(),
			}, nil
		}
		collect := startJob(t, c, vnodes.VirtualNode{
			Name:     "fixture",
			Hostname: "fixture",
			GUID:     "245dea48-ae2a-4697-a22e-050e7e358afb",
		})
		output := collect(c.UpdateEvery)
		samples := parseSamples(t, output)

		reached := make(map[string][]string)
		for id, chart := range parseCharts(t, output) {
			if !strings.HasPrefix(chart.context, "wmi.smb_server_") {
				continue
			}
			for _, dimension := range chart.dimensions {
				where := chart.context + " " + dimension.name
				value, ok := samples[id][dimension.name]
				require.True(t, ok, "%s has no value", where)
				property, ok := properties[value]
				require.True(t, ok, "%s shows %v, no Server counter's value", where, value)
				reached[property] = append(reached[property], where)

				algorithm := "incremental"
				if pointValues[property] {
					algorithm = "absolute"
				}
				assert.Equal(t, algorithm, dimension.algorithm, where)
				assert.Equal(t, 1, max(dimension.multiplier, -dimension.multiplier), where)
				assert.Equal(t, 1, dimension.divisor, where)
			}
		}
		for property := range row {
			assert.Len(t, reached[property], 1, "%s reaches %v", property, reached[property])
		}
	})
}

// fixtureNumber reads a fixture's counter value as DCOM (unsigned integers) or
// WinRM (decimal text) returns it.
func fixtureNumber(t *testing.T, value any) float64 {
	t.Helper()
	switch v := value.(type) {
	case uint32:
		return float64(v)
	case uint64:
		return float64(v)
	case string:
		n, err := strconv.ParseUint(v, 10, 64)
		require.NoError(t, err)
		return float64(n)
	}
	require.Failf(t, "unexpected fixture value", "%T", value)
	return 0
}

// parseSamples reads the collected values from plugin protocol output, by
// chart ID and dimension name, or by $name for a chart variable.
func parseSamples(t *testing.T, output string) map[string]map[string]float64 {
	t.Helper()
	samples := make(map[string]map[string]float64)
	var current map[string]float64
	for line := range strings.Lines(output) {
		var fields []string
		for _, match := range protocolField.FindAllStringSubmatch(line, -1) {
			fields = append(fields, match[1]+match[2])
		}
		switch {
		case len(fields) >= 2 && fields[0] == "BEGIN":
			current = make(map[string]float64)
			samples[fields[1]] = current
		case len(fields) >= 1 && fields[0] == "END":
			current = nil
		case len(fields) == 4 && fields[0] == "SET" && current != nil:
			value, err := strconv.ParseInt(fields[3], 10, 64)
			require.NoError(t, err, line)
			current[fields[1]] = float64(value)
		case len(fields) == 5 && fields[0] == "VARIABLE" && current != nil:
			value, err := strconv.ParseFloat(fields[4], 64)
			require.NoError(t, err, line)
			current["$"+fields[2]] = value
		}
	}
	return samples
}

// TestMetadataDocumentsChartLabels requires every metadata scope to document
// exactly the labels the job publishes on the charts of its metrics. A label
// that some instances leave out, such as a disk's mount point, counts once.
func TestMetadataDocumentsChartLabels(t *testing.T) {
	published := make(map[string][]string)
	for _, chart := range publishedCharts(t) {
		keys := slices.AppendSeq(published[chart.context], maps.Keys(chart.labels))
		slices.Sort(keys)
		published[chart.context] = slices.Compact(keys)
	}
	for context, keys := range published {
		if keys == nil {
			published[context] = []string{}
		}
	}

	data, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	var doc struct {
		Modules []struct {
			Metrics struct {
				Scopes []struct {
					Labels []struct {
						Name string `yaml:"name"`
					} `yaml:"labels"`
					Metrics []struct {
						Name string `yaml:"name"`
					} `yaml:"metrics"`
				} `yaml:"scopes"`
			} `yaml:"metrics"`
		} `yaml:"modules"`
	}
	require.NoError(t, yaml.Unmarshal(data, &doc))
	require.Len(t, doc.Modules, 1)

	documented := make(map[string][]string)
	for _, scope := range doc.Modules[0].Metrics.Scopes {
		labels := []string{}
		for _, label := range scope.Labels {
			labels = append(labels, label.Name)
		}
		slices.Sort(labels)
		for _, metric := range scope.Metrics {
			documented[metric.Name] = labels
		}
	}
	assert.Equal(t, published, documented)
}
