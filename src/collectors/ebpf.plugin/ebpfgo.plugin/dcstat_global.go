package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

const (
	dcstatGlobalGroup  = "filesystem"
	dcstatGlobalFamily = "directory_cache"
	dcstatGlobalModule = "dcstat"
	dcstatGlobalPlugin = "ebpf-go.plugin"
)

type dcstatGlobalCounters struct {
	Reference uint64
	Slow      uint64
	Miss      uint64
}

type dcstatGlobalPublish struct {
	Ratio     int64
	Reference int64
	Slow      int64
	Miss      int64
}

type dcstatGlobalState struct {
	initialized bool
	prev        dcstatGlobalCounters
}

type dcstatGlobalDimension struct {
	id        string
	algorithm string
}

type dcstatGlobalChart struct {
	id         string
	title      string
	units      string
	context    string
	order      int
	dimensions []dcstatGlobalDimension
}

// Chart ids, contexts, units, and priorities are the ones the C dcstat module
// published, so existing dashboards and alarms keep working after the port.
var dcstatGlobalCharts = []dcstatGlobalChart{
	{
		id:      "dc_hit_ratio",
		title:   "Percentage of files inside directory cache",
		units:   "%",
		context: "filesystem.dc_hit_ratio",
		order:   21200,
		dimensions: []dcstatGlobalDimension{
			{id: "ratio", algorithm: "absolute"},
		},
	},
	{
		id:    "dc_reference",
		title: "Variables used to calculate hit ratio.",
		// The dimensions are cumulative counters published with the incremental
		// algorithm, so the database renders them as a rate.
		units:   "files/s",
		context: "filesystem.dc_reference",
		order:   21201,
		dimensions: []dcstatGlobalDimension{
			{id: "reference", algorithm: "incremental"},
			{id: "slow", algorithm: "incremental"},
			{id: "miss", algorithm: "incremental"},
		},
	},
}

var dcstatGlobalChartsOnce sync.Once

// dcstatHitRatio converts one interval's directory-cache deltas into the hit
// ratio percentage.  It is the single definition of dcstat's ratio semantics:
// the global collector and the per-PID shared-memory rows both call it, so the
// two can never drift apart.
//
// The idle convention is the C collector's: no lookups this interval means a
// ratio of 0.  cachestat deliberately reports 100 for its idle case; the two
// metrics are not interchangeable.
func dcstatHitRatio(reference, notFound int64) int64 {
	if reference <= 0 {
		return 0
	}

	successful := max(reference-notFound, 0)
	return int64((float64(successful) / float64(reference)) * 100)
}

// Update computes the publish values for one collection cycle.
//
// reference/slow/miss are published as the raw cumulative BPF counters with the
// `incremental` algorithm, so the database derives the rate.  The ratio is a
// per-interval value (the C collector divided lifetime totals, which barely
// moves after a few hours of uptime and disagrees with the per-app and
// per-cgroup ratios computed from interval deltas).
func (s *dcstatGlobalState) Update(current dcstatGlobalCounters) (dcstatGlobalPublish, bool) {
	publish := dcstatGlobalPublish{
		Reference: int64(current.Reference),
		Slow:      int64(current.Slow),
		Miss:      int64(current.Miss),
	}

	if s.initialized {
		publish.Ratio = dcstatHitRatio(
			diffCounters(current.Reference, s.prev.Reference),
			diffCounters(current.Miss, s.prev.Miss),
		)
	}

	s.prev = current
	s.initialized = true

	return publish, true
}

func createDCStatGlobalCharts(api *netdataapi.API, updateEvery int) {
	dcstatGlobalChartsOnce.Do(func() {
		pluginOutputMu.Lock()
		defer pluginOutputMu.Unlock()

		if api != nil {
			api.HOST("")
		}
		for _, chart := range dcstatGlobalCharts {
			emitDCStatGlobalChart(api, chart, updateEvery)
		}
	})
}

func emitDCStatGlobalChart(api *netdataapi.API, chart dcstatGlobalChart, updateEvery int) {
	if api == nil {
		return
	}

	api.CHART(netdataapi.ChartOpts{
		TypeID:      dcstatGlobalGroup,
		ID:          chart.id,
		Title:       chart.title,
		Units:       chart.units,
		Family:      dcstatGlobalFamily,
		Context:     chart.context,
		ChartType:   "line",
		Priority:    chart.order,
		UpdateEvery: updateEvery,
		Plugin:      dcstatGlobalPlugin,
		Module:      dcstatGlobalModule,
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

func (p dcstatGlobalPublish) write(api *netdataapi.API, usecSince int) {
	if api == nil {
		return
	}

	pluginOutputMu.Lock()
	defer pluginOutputMu.Unlock()

	api.BEGIN(dcstatGlobalGroup, "dc_hit_ratio", usecSince)
	api.SET("ratio", p.Ratio)
	api.END()

	api.BEGIN(dcstatGlobalGroup, "dc_reference", usecSince)
	api.SET("reference", p.Reference)
	api.SET("slow", p.Slow)
	api.SET("miss", p.Miss)
	api.END()
}

// runDCStatGlobalCollector is dcstat's single collection loop.  The global
// metric snapshot and the per-PID SHM publish both run here sequentially so
// only one OS thread is needed for the CGO calls.
//
// store may be nil when apps/cgroups integration is disabled.  shouldPublish is
// true only when dcstat owns the shared-memory segment: exactly one module
// publishes it (see main.go), and every other module just contributes rows.
func runDCStatGlobalCollector(
	api *netdataapi.API,
	handle *DCStatLegacyHandle,
	stop <-chan struct{},
	store *ebpfSharedMemoryStore,
	updateEvery int,
	shouldPublish bool,
) {
	if handle == nil || handle.Runtime == nil {
		return
	}

	if updateEvery <= 0 {
		updateEvery = dcstatDefaultUpdateEvery
	}

	createDCStatGlobalCharts(api, updateEvery)

	state := &dcstatGlobalState{}
	lastCollection := time.Now()
	collectAndPublish := func(usecSince int) {
		// Global snapshot — one CGO call.
		snapshot, err := handle.Runtime.Snapshot(handle.MapsPerCore)
		if err != nil {
			logPluginErr("dcstat.snapshot", "dcstat", "snapshot", err)
		} else {
			publish, ok := state.Update(dcstatGlobalCounters{
				Reference: snapshot.Reference,
				Slow:      snapshot.Slow,
				Miss:      snapshot.Miss,
			})
			if ok {
				publish.write(api, usecSince)
			}
		}

		// Per-PID snapshot — second CGO call, same goroutine, no extra thread.
		if store == nil {
			return
		}

		apps, err := handle.Runtime.SnapshotApps(handle.MapsPerCore)
		if err != nil {
			logPluginErr("dcstat.snapshot", "dcstat", "snapshot-apps", err)
			// No valid per-PID data this cycle.  Drop dcstat's rows as well as its
			// flag: when another module owns the segment we cannot stamp the header
			// ourselves, so the cleared state has to already be in the store for
			// the owner's next publish to carry it.  Consumers gate on the flag,
			// and the owner publishes on its own interval, so the window in which
			// they can still see the previous header is bounded by that interval.
			store.ClearDCStatApps()
			if shouldPublish && handle.SharedMemory != nil {
				if perr := store.Publish(handle.SharedMemory, ebpfgoSHMFlagDCStat); perr != nil {
					logPluginErr("dcstat.publish", "dcstat", "shared memory publish", perr)
				}
			}
			return
		}

		staleCandidates := store.UpdateDCStatApps(apps)
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
					rateLimitedStderr("dcstat.delete_pids",
						fmt.Sprintf("ebpf-go.plugin: failed to delete %d stale PIDs from dcstat_pid: %v\n",
							len(deadPIDs), err))
				}
			}
		}

		if !shouldPublish {
			return
		}

		// Lazy SHM open: allocate the publisher on the first cycle that has a
		// non-empty store so the default config (no apps, no cgroups) does not
		// pay the VMA cost.  The handle is mutated under the loop's
		// single-goroutine guarantee so no extra lock is needed.
		if handle.SharedMemory == nil {
			publisher, perr := NewSharedPidMemoryPublisher(productionSHMName, productionSEMName, handle.PidTableSize, uint32(updateEvery))
			if perr != nil {
				logPluginErr("dcstat.shm_open", "dcstat", "shared memory open", perr)
			} else {
				handle.SharedMemory = publisher
			}
		}
		if handle.SharedMemory != nil {
			if err := store.Publish(handle.SharedMemory, ebpfgoSHMFlagDCStat); err != nil {
				logPluginErr("dcstat.publish", "dcstat", "shared memory publish", err)
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
