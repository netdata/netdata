package main

import (
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// fdSHMFlags is the flag mask fd owns in the shared-memory header.  Publish uses
// it to clear fd's own bits after each cycle, so both must be listed: the errors
// bit is fd's to set and clear, not a second module's.
const fdSHMFlags = ebpfgoSHMFlagFD | ebpfgoSHMFlagFDErrors

const (
	fdGlobalGroup  = "filesystem"
	fdGlobalFamily = "file_access"
	fdGlobalModule = "filedescriptor"
	fdGlobalPlugin = "ebpf-go.plugin"
)

type fdGlobalDimension struct {
	id        string
	algorithm string
}

type fdGlobalChart struct {
	id         string
	title      string
	units      string
	context    string
	order      int
	dimensions []fdGlobalDimension
	// errorChart marks a chart that only exists in `return` mode, mirroring the
	// C module's `em->mode < MODE_ENTRY` gate.
	errorChart bool
}

// Chart ids, contexts, units, priorities and algorithms match the C
// filedescriptor module.  The dimensions carry the raw cumulative BPF counters
// and the `incremental` algorithm turns them into calls/s, exactly as before —
// unlike dcstat, whose C global chart was already `absolute`.
var fdGlobalCharts = []fdGlobalChart{
	{
		id:      "file_descriptor",
		title:   "Open and close calls",
		units:   "calls/s",
		context: "filesystem.file_descriptor",
		order:   20270,
		dimensions: []fdGlobalDimension{
			{id: "open", algorithm: "incremental"},
			{id: "close", algorithm: "incremental"},
		},
	},
	{
		id:      "file_error",
		title:   "Open fails",
		units:   "calls/s",
		context: "filesystem.file_error",
		order:   20271,
		dimensions: []fdGlobalDimension{
			{id: "open", algorithm: "incremental"},
			{id: "close", algorithm: "incremental"},
		},
		errorChart: true,
	},
}

var fdGlobalChartsOnce sync.Once

func createFDGlobalCharts(api *netdataapi.API, updateEvery int, reportErrors bool) {
	fdGlobalChartsOnce.Do(func() {
		pluginOutputMu.Lock()
		defer pluginOutputMu.Unlock()

		if api != nil {
			api.HOST("")
		}
		for _, chart := range fdGlobalCharts {
			if chart.errorChart && !reportErrors {
				continue
			}
			emitFDGlobalChart(api, chart, updateEvery)
		}
	})
}

func emitFDGlobalChart(api *netdataapi.API, chart fdGlobalChart, updateEvery int) {
	if api == nil {
		return
	}

	api.CHART(netdataapi.ChartOpts{
		TypeID:      fdGlobalGroup,
		ID:          chart.id,
		Title:       chart.title,
		Units:       chart.units,
		Family:      fdGlobalFamily,
		Context:     chart.context,
		ChartType:   "line",
		Priority:    chart.order,
		UpdateEvery: updateEvery,
		Plugin:      fdGlobalPlugin,
		Module:      fdGlobalModule,
	})
	for _, dim := range chart.dimensions {
		api.DIMENSION(netdataapi.DimensionOpts{
			ID:         dim.id,
			Name:       dim.id,
			Algorithm:  dim.algorithm,
			Multiplier: 1,
			Divisor:    1,
		})
	}
}

// writeFDGlobal emits the cumulative counters.  No delta is computed here: the
// dimensions are `incremental`, so the agent derives the rate itself and a
// restart of this plugin cannot produce a spike.
func writeFDGlobal(api *netdataapi.API, snapshot libbpfloader.FDSnapshot, usecSince int, reportErrors bool) {
	if api == nil {
		return
	}

	pluginOutputMu.Lock()
	defer pluginOutputMu.Unlock()

	api.BEGIN(fdGlobalGroup, "file_descriptor", usecSince)
	api.SET("open", int64(snapshot.OpenCall))
	api.SET("close", int64(snapshot.CloseCall))
	api.END()

	if !reportErrors {
		return
	}

	api.BEGIN(fdGlobalGroup, "file_error", usecSince)
	api.SET("open", int64(snapshot.OpenErr))
	api.SET("close", int64(snapshot.CloseErr))
	api.END()
}

// runFDGlobalCollector is fd's single collection loop.  The global metric
// snapshot and the per-PID SHM publish both run here sequentially so only one OS
// thread is needed for the CGO calls.
//
// store may be nil when apps/cgroups integration is disabled.  shouldPublish is
// true only when fd owns the shared-memory segment: exactly one module publishes
// it (see main.go), and every other module just contributes rows.
func runFDGlobalCollector(
	api *netdataapi.API,
	handle *FDLegacyHandle,
	stop <-chan struct{},
	store *ebpfSharedMemoryStore,
	updateEvery int,
	shouldPublish bool,
) {
	if handle == nil || handle.Runtime == nil {
		return
	}

	if updateEvery <= 0 {
		updateEvery = fdDefaultUpdateEvery
	}

	createFDGlobalCharts(api, updateEvery, handle.ReportErrors)

	lastCollection := time.Now()
	collectAndPublish := func(usecSince int) {
		// Global snapshot — one CGO call.
		snapshot, err := handle.Runtime.Snapshot(handle.MapsPerCore)
		if err != nil {
			logPluginErr("fd.snapshot", "fd", "snapshot", err)
		} else {
			writeFDGlobal(api, snapshot, usecSince, handle.ReportErrors)
		}

		// Per-PID snapshot — second CGO call, same goroutine, no extra thread.
		if store == nil {
			return
		}

		apps, err := handle.Runtime.SnapshotApps(handle.MapsPerCore)
		if err != nil {
			logPluginErr("fd.snapshot_apps", "fd", "snapshot-apps", err)
			// No valid per-PID data this cycle.  Drop fd's rows as well as its
			// flag: when another module owns the segment we cannot stamp the header
			// ourselves, so the cleared state has to already be in the store for
			// the owner's next publish to carry it.  Consumers gate on the flag,
			// and the owner publishes on its own interval, so the window in which
			// they can still see the previous header is bounded by that interval.
			store.ClearFDApps()
			if shouldPublish && handle.SharedMemory != nil {
				if perr := store.Publish(handle.SharedMemory, fdSHMFlags); perr != nil {
					logPluginErr("fd.publish", "fd", "shared memory publish", perr)
				}
			}
			return
		}

		staleCandidates := store.UpdateFDApps(apps, handle.ReportErrors)
		if len(staleCandidates) > 0 {
			// Authoritative liveness check matching the C-version behavior: a
			// process is alive iff kill(pid, 0) succeeds.  Idle-but-alive PIDs
			// stay in the BPF map so their next event is still attributable.
			deadPIDs := staleCandidates[:0]
			for _, pid := range staleCandidates {
				if !libbpfloader.PidIsAlive(pid) {
					deadPIDs = append(deadPIDs, pid)
				}
			}
			if len(deadPIDs) > 0 {
				if err := handle.Runtime.DeletePids(deadPIDs); err != nil {
					rateLimitedStderr("fd.delete_pids",
						"ebpf-go.plugin: failed to delete %d stale PIDs from tbl_fd_pid: %v\n",
						len(deadPIDs), err)
				} else {
					store.RemoveFDPIDs(deadPIDs)
				}
			}
		}

		if !shouldPublish {
			return
		}

		// Lazy SHM open: allocate the publisher on the first cycle that reaches
		// here, so the default config (no apps, no cgroups) never pays the VMA
		// cost — main.go leaves store nil in that case and the loop returns
		// above.  The handle is mutated under the loop's single-goroutine
		// guarantee so no extra lock is needed.
		if handle.SharedMemory == nil {
			publisher, perr := NewSharedPidMemoryPublisher(productionSHMName, productionSEMName, handle.PidTableSize, uint32(updateEvery))
			if perr != nil {
				logPluginErr("fd.shm_open", "fd", "shared memory open", perr)
			} else {
				handle.SharedMemory = publisher
			}
		}
		if handle.SharedMemory != nil {
			if err := store.Publish(handle.SharedMemory, fdSHMFlags); err != nil {
				logPluginErr("fd.publish", "fd", "shared memory publish", err)
			}
		}
	}

	collectAndPublish(0)
	lastCollection = time.Now()

	ticker := time.NewTicker(time.Duration(updateEvery) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		}

		now := time.Now()
		usecSince := max(int(now.Sub(lastCollection).Microseconds()), 0)
		lastCollection = now
		collectAndPublish(usecSince)
	}
}
