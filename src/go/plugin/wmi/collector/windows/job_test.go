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
	return results
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
// src/collectors/windows.plugin/perflib-network.c (system.net and the net.*
// interface charts) and
// src/collectors/windows.plugin/perflib-storage.c (disk.space). The ping.*
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
		instance("ping.host_rtt", map[string]string{"host": "windows.example.com"}),
		instance("ping.host_std_dev_rtt", map[string]string{"host": "windows.example.com"}),
		instance("ping.host_packet_loss", map[string]string{"host": "windows.example.com"}),
	}
	for context := range native {
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
	}
	published := publishedCharts(t)
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

// TestCollector_SystemCountersFeedNativeDimensions checks that each System
// counter reaches the native chart dimension that reads it
// (src/collectors/windows.plugin/perflib-processes.c, do_processes and
// do_processor_queue), and that uptime arrives in milliseconds since boot, as
// GetSystemUptime.c sends GetTickCount64().
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
// exactly the labels the job publishes on the charts of its metrics.
func TestMetadataDocumentsChartLabels(t *testing.T) {
	published := make(map[string][]string)
	for _, chart := range publishedCharts(t) {
		keys := slices.AppendSeq([]string{}, maps.Keys(chart.labels))
		slices.Sort(keys)
		if previous, ok := published[chart.context]; ok {
			require.Equal(t, previous, keys, "labels of %s instances", chart.context)
		}
		published[chart.context] = keys
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
