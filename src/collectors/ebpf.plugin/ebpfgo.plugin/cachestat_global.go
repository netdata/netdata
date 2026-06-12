package main

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

const (
	cachestatGlobalGroup  = "mem"
	cachestatGlobalFamily = "page_cache"
	cachestatGlobalModule = "cachestat"
	cachestatGlobalPlugin = "ebpf-go.plugin"
)

type cachestatGlobalCounters struct {
	MarkPageAccessed   uint64
	MarkBufferDirty    uint64
	AddToPageCacheLru  uint64
	AccountPageDirtied uint64
}

type cachestatGlobalPublish struct {
	Ratio int64
	Dirty int64
	Hit   int64
	Miss  int64
}

type cachestatGlobalState struct {
	initialized bool
	prev        cachestatGlobalCounters
	cumDirty    int64
	cumHits     int64
	cumMisses   int64
}

type cachestatGlobalChart struct {
	id        string
	title     string
	units     string
	context   string
	order     int
	dimension string
	algorithm string
}

var cachestatGlobalCharts = []cachestatGlobalChart{
	{
		id:        "cachestat_ratio",
		title:     "Hit ratio",
		units:     "%",
		context:   "mem.cachestat_ratio",
		order:     21100,
		dimension: "ratio",
		algorithm: "absolute",
	},
	{
		id:        "cachestat_dirties",
		title:     "Number of dirty pages",
		units:     "page/s",
		context:   "mem.cachestat_dirties",
		order:     21101,
		dimension: "dirty",
		algorithm: "incremental",
	},
	{
		id:        "cachestat_hits",
		title:     "Number of accessed files",
		units:     "hits/s",
		context:   "mem.cachestat_hits",
		order:     21102,
		dimension: "hit",
		algorithm: "incremental",
	},
	{
		id:        "cachestat_misses",
		title:     "Files out of page cache",
		units:     "misses/s",
		context:   "mem.cachestat_misses",
		order:     21103,
		dimension: "miss",
		algorithm: "incremental",
	},
}

var cachestatGlobalChartsOnce sync.Once
var cachestatStdoutMutex sync.Mutex

// cachestatErrorLogInterval is the minimum gap between repeated stderr
// messages from a single error site.  A persistent failure (e.g. unhealthy
// BPF map) would otherwise emit one line per collection cycle, flooding
// the operator log; 60 s strikes a balance between visibility and noise.
const cachestatErrorLogInterval = 60 * time.Second

var (
	cachestatErrorMu      sync.Mutex
	cachestatErrorLastLog = map[string]time.Time{}
)

// rateLimitedStderr writes msg to stderr the first time and at most once per
// cachestatErrorLogInterval.  The site key identifies the error site; use a
// short stable string per call site.
func rateLimitedStderr(site, msg string) {
	cachestatErrorMu.Lock()
	defer cachestatErrorMu.Unlock()

	now := time.Now()
	if last, ok := cachestatErrorLastLog[site]; ok && now.Sub(last) < cachestatErrorLogInterval {
		return
	}
	cachestatErrorLastLog[site] = now
	fmt.Fprint(os.Stderr, msg)
}

func (s *cachestatGlobalState) Update(current cachestatGlobalCounters) (cachestatGlobalPublish, bool) {
	mpa := diffCounters(current.MarkPageAccessed, s.prev.MarkPageAccessed)
	mbd := diffCounters(current.MarkBufferDirty, s.prev.MarkBufferDirty)
	apcl := diffCounters(current.AddToPageCacheLru, s.prev.AddToPageCacheLru)
	apd := diffCounters(current.AccountPageDirtied, s.prev.AccountPageDirtied)

	publish := cachestatGlobalPublish{}

	total := mpa - mbd
	if total < 0 {
		total = 0
	}

	misses := apcl - apd
	if misses < 0 {
		misses = 0
	}

	hits := total - misses
	if hits < 0 {
		misses = total
		hits = 0
	}

	if total > 0 {
		publish.Ratio = int64((float64(hits) / float64(total)) * 100)
	} else {
		publish.Ratio = 100
	}

	s.cumDirty += mbd
	s.cumHits += hits
	s.cumMisses += misses

	publish.Dirty = s.cumDirty
	publish.Hit = s.cumHits
	publish.Miss = s.cumMisses
	s.prev = current
	s.initialized = true

	return publish, true
}

func diffCounters(current, previous uint64) int64 {
	if current < previous {
		return 0
	}

	return int64(current - previous)
}

func createCachestatGlobalCharts(api *netdataapi.API, updateEvery int) {
	cachestatGlobalChartsOnce.Do(func() {
		cachestatStdoutMutex.Lock()
		defer cachestatStdoutMutex.Unlock()

		if api != nil {
			api.HOST("")
		}
		for _, chart := range cachestatGlobalCharts {
			emitCachestatGlobalChart(api, chart, updateEvery)
		}
	})
}

func emitCachestatGlobalChart(api *netdataapi.API, chart cachestatGlobalChart, updateEvery int) {
	if api == nil {
		return
	}

	api.CHART(netdataapi.ChartOpts{
		TypeID:      cachestatGlobalGroup,
		ID:          chart.id,
		Title:       chart.title,
		Units:       chart.units,
		Family:      cachestatGlobalFamily,
		Context:     chart.context,
		ChartType:   "line",
		Priority:    chart.order,
		UpdateEvery: updateEvery,
		Plugin:      cachestatGlobalPlugin,
		Module:      cachestatGlobalModule,
	})
	api.DIMENSION(netdataapi.DimensionOpts{
		ID:         chart.dimension,
		Name:       chart.dimension,
		Algorithm:  chart.algorithm,
		Multiplier: 1,
		Divisor:    1,
	})
}

func (p cachestatGlobalPublish) write(api *netdataapi.API, usecSince int) {
	if api == nil {
		return
	}

	cachestatStdoutMutex.Lock()
	defer cachestatStdoutMutex.Unlock()

	for _, item := range []struct {
		chart string
		dim   string
		value int64
	}{
		{chart: "cachestat_ratio", dim: "ratio", value: p.Ratio},
		{chart: "cachestat_dirties", dim: "dirty", value: p.Dirty},
		{chart: "cachestat_hits", dim: "hit", value: p.Hit},
		{chart: "cachestat_misses", dim: "miss", value: p.Miss},
	} {
		api.BEGIN(cachestatGlobalGroup, item.chart, usecSince)
		api.SET(item.dim, item.value)
		api.END()
	}
}

// runCachestatGlobalCollector is the single collection loop for the plugin.
// Both the global metric snapshot and the per-PID SHM publish run here
// sequentially so that only one OS thread is needed for CGO calls.
// store may be nil when apps/cgroups integration is disabled.
func runCachestatGlobalCollector(api *netdataapi.API, handle *CachestatLegacyHandle, stop <-chan struct{}, store *cachestatSharedMemoryStore, updateEvery int) {
	if handle == nil || handle.Runtime == nil {
		return
	}

	if updateEvery <= 0 {
		updateEvery = cachestatDefaultUpdateEvery
	}

	createCachestatGlobalCharts(api, updateEvery)

	state := &cachestatGlobalState{}
	lastCollection := time.Now()
	collectAndPublish := func(usecSince int) {
		// Global snapshot — one CGO call.
		snapshot, err := handle.Runtime.Snapshot(handle.MapsPerCore)
		if err != nil {
			rateLimitedStderr("cachestat.snapshot",
				fmt.Sprintf("ebpf-go.plugin: cachestat snapshot failed: %v\n", err))
		} else {
			publish, ok := state.Update(cachestatGlobalCounters{
				MarkPageAccessed:   snapshot.MarkPageAccessed,
				MarkBufferDirty:    snapshot.MarkBufferDirty,
				AddToPageCacheLru:  snapshot.AddToPageCacheLru,
				AccountPageDirtied: snapshot.AccountPageDirtied,
			})
			if ok {
				publish.write(api, usecSince)
			}
		}

		// Per-PID snapshot — second CGO call, same goroutine, no extra thread.
		if store != nil {
			apps, err := handle.Runtime.SnapshotApps(handle.MapsPerCore)
			if err == nil {
				staleCandidates := store.UpdateApps(apps)
				if len(staleCandidates) > 0 {
					// Authoritative liveness check matching the C-version
					// behavior: a process is alive iff kill(pid, 0) succeeds.
					// Idle-but-alive PIDs are kept in the BPF map so their
					// next BPF event is still attributable to the process.
					// We reset the debouncer by going through the store once
					// more only for the dead candidates (see filter below).
					deadPIDs := staleCandidates[:0]
					for _, pid := range staleCandidates {
						if !libbpfloader.PidIsAlive(pid) {
							deadPIDs = append(deadPIDs, pid)
						}
					}
					if len(deadPIDs) > 0 {
						if err := handle.Runtime.DeletePids(deadPIDs); err != nil {
							rateLimitedStderr("cachestat.delete_pids",
								fmt.Sprintf("ebpf-go.plugin: failed to delete %d stale PIDs from cstat_pid: %v\n",
									len(deadPIDs), err))
						}
					}
				}
				// Lazy SHM open: allocate the publisher on the first
				// cycle that has a non-empty store so the default config
				// (no apps, no cgroups) does not pay the 17.5 MB VMA
				// cost.  The handle is mutated under the loop's single-
				// goroutine guarantee so no extra lock is needed.
				if handle.SharedMemory == nil {
					publisher, perr := NewSharedPidMemoryPublisher(handle.PidTableSize)
					if perr != nil {
						rateLimitedStderr("cachestat.shm_open",
							fmt.Sprintf("ebpf-go.plugin: cachestat shared memory open failed: %v\n", perr))
					} else {
						handle.SharedMemory = publisher
					}
				}
				if handle.SharedMemory != nil {
					if err := store.Publish(handle.SharedMemory); err != nil {
						rateLimitedStderr("cachestat.publish",
							fmt.Sprintf("ebpf-go.plugin: cachestat shared memory publish failed: %v\n", err))
					}
				}
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
		usecSince := int(now.Sub(lastCollection).Microseconds())
		if usecSince < 0 {
			usecSince = 0
		}
		lastCollection = now
		collectAndPublish(usecSince)
	}
}

