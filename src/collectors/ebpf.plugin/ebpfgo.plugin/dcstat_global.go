package main

import (
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
	prev dcstatGlobalCounters
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

// Chart ids, contexts, units, and algorithms match the C dcstat module. The
// count dimensions are interval totals, while the ratio is attach-lifetime.
var dcstatGlobalCharts = []dcstatGlobalChart{
	{
		id:      "dc_hit_ratio",
		title:   "Percentage of directory lookups resolved by the cache",
		units:   "%",
		context: "filesystem.dc_hit_ratio",
		order:   21200,
		dimensions: []dcstatGlobalDimension{
			{id: "ratio", algorithm: "absolute"},
		},
	},
	{
		id:      "dc_reference",
		title:   "Variables used to calculate hit ratio.",
		units:   "files",
		context: "filesystem.dc_reference",
		order:   21201,
		dimensions: []dcstatGlobalDimension{
			{id: "reference", algorithm: "absolute"},
			{id: "slow", algorithm: "absolute"},
			{id: "miss", algorithm: "absolute"},
		},
	},
}

var dcstatGlobalChartsOnce sync.Once

// dcstatHitRatio converts per-PID directory-cache interval counters into a
// hit-ratio percentage for shared-memory consumers.
//
// The idle convention is the C collector's: no supplied lookups means a ratio
// of 0. cachestat deliberately reports 100 for its idle case.
func dcstatHitRatio(reference, notFound int64) int64 {
	if reference <= 0 {
		return 0
	}

	successful := max(reference-notFound, 0)
	return int64((float64(successful) / float64(reference)) * 100)
}

// dcstatCumulativeHitRatio preserves the legacy global chart's attach-lifetime
// semantics without narrowing its uint64 BPF counters to int64 first.
func dcstatCumulativeHitRatio(reference, notFound uint64) int64 {
	if reference == 0 {
		return 0
	}
	if notFound > reference {
		notFound = reference
	}

	return int64((float64(reference-notFound) / float64(reference)) * 100)
}

// Update computes the publish values for one collection cycle.
//
// Count values are interval deltas, published with the `absolute` algorithm.
// The ratio uses raw cumulative counters, as the legacy global chart did.
func (s *dcstatGlobalState) Update(current dcstatGlobalCounters) (dcstatGlobalPublish, bool) {
	publish := dcstatGlobalPublish{
		Ratio:     dcstatCumulativeHitRatio(current.Reference, current.Miss),
		Reference: diffCounters(current.Reference, s.prev.Reference),
		Slow:      diffCounters(current.Slow, s.prev.Slow),
		Miss:      diffCounters(current.Miss, s.prev.Miss),
	}

	s.prev = current

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
			logPluginErr("dcstat.snapshot_apps", "dcstat", "snapshot-apps", err)
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

		staleCandidates := store.UpdateDCStatApps(apps, uint32(updateEvery))
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
						"ebpf-go.plugin: failed to delete %d stale PIDs from dcstat_pid: %v\n",
						len(deadPIDs), err)
				} else {
					store.RemoveDCStatPIDs(deadPIDs)
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
