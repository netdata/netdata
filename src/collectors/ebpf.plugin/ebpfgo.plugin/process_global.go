package main

import (
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// processSHMFlags is the flag mask process owns in the shared-memory header.
const processSHMFlags = ebpfgoSHMFlagProcess | ebpfgoSHMFlagProcessErrors

const (
	processGlobalGroup  = "system"
	processGlobalFamily = "processes"
	processGlobalModule = "process"
	processGlobalPlugin = "ebpf-go.plugin"
)

// Dimension ids are the C module's kernel-function ids (process_id_names in the
// removed ebpf_process.c); the names are what the dashboard shows.
type processGlobalDimension struct {
	id        string
	name      string
	algorithm string
}

type processGlobalChart struct {
	id         string
	title      string
	units      string
	context    string
	order      int
	dimensions []processGlobalDimension
	// errorChart marks a chart that only exists in `return` mode, mirroring the
	// C module's `em->mode < MODE_ENTRY` gate.
	errorChart bool
}

// Chart ids, contexts, units, priorities, dimensions and algorithms match the C
// process module, so existing chart instances and dashboards keep resolving.
// Like dcstat, and unlike fd, the C module published per-interval differences
// with the `absolute` algorithm (ebpf_update_global_publish and the ABSOLUTE
// entries passed to ebpf_global_labels in the removed ebpf_process.c), so the
// "calls/s" charts carry calls per collection interval.  RHEL 7 kernels used
// `do_fork` instead of `_do_fork`; every other kernel used `_do_fork`.
var processGlobalCharts = []processGlobalChart{
	{
		id: "process_thread", title: "Start process", units: "calls/s",
		context: "system.process_thread", order: 21002,
		dimensions: []processGlobalDimension{{"_do_fork", "process", "absolute"}, {"sys_clone", "thread", "absolute"}},
	},
	{
		id: "exit", title: "Exit process", units: "calls/s",
		context: "system.exit", order: 21003,
		dimensions: []processGlobalDimension{{"do_exit", "process", "absolute"}, {"release_task", "task", "absolute"}},
	},
	{
		id: "process_status", title: "Process not closed", units: "calls",
		context: "system.process_status", order: 21004,
		dimensions: []processGlobalDimension{{"process", "process", "absolute"}, {"zombie", "zombie", "absolute"}},
	},
	{
		id: "task_error", title: "Fails to create process", units: "calls/s",
		context: "system.task_error", order: 21005,
		dimensions: []processGlobalDimension{{"_do_fork", "process", "absolute"}, {"sys_clone", "thread", "absolute"}},
		errorChart: true,
	},
}

var processGlobalChartsOnce sync.Once

func createProcessGlobalCharts(api *netdataapi.API, updateEvery int, reportErrors bool) {
	if api == nil {
		return
	}

	processGlobalChartsOnce.Do(func() {
		pluginOutputMu.Lock()
		defer pluginOutputMu.Unlock()

		api.HOST("")
		for _, chart := range processGlobalCharts {
			if chart.errorChart && !reportErrors {
				continue
			}
			api.CHART(netdataapi.ChartOpts{
				TypeID:      processGlobalGroup,
				ID:          chart.id,
				Title:       chart.title,
				Units:       chart.units,
				Family:      processGlobalFamily,
				Context:     chart.context,
				ChartType:   "line",
				Priority:    chart.order,
				UpdateEvery: updateEvery,
				Plugin:      processGlobalPlugin,
				Module:      processGlobalModule,
			})
			for _, dim := range chart.dimensions {
				api.DIMENSION(netdataapi.DimensionOpts{
					ID: dim.id, Name: dim.name, Algorithm: dim.algorithm, Multiplier: 1, Divisor: 1,
				})
			}
		}
	})
}

// processInterval is one collection interval's counter differences.  running
// and zombie are the C module's "not closed" view: tasks created that are not
// threads, and tasks that exited but were not yet released.
type processInterval struct {
	exits, taskClose, process, processErr, thread, threadErr int64
	running, zombie                                          int64
}

// processGlobalState keeps the previous snapshot.  It starts at zero, as the C
// module's did, so the first interval reports the counts since the programs
// were attached.
type processGlobalState struct {
	prev libbpfloader.ProcessSnapshot
}

func (s *processGlobalState) interval(current libbpfloader.ProcessSnapshot) processInterval {
	prev := s.prev
	s.prev = current

	in := processInterval{
		exits:      diffCounters(current.Exits, prev.Exits),
		taskClose:  diffCounters(current.TaskClose, prev.TaskClose),
		process:    diffCounters(current.Process, prev.Process),
		processErr: diffCounters(current.ProcessErr, prev.ProcessErr),
		thread:     diffCounters(current.Thread, prev.Thread),
		threadErr:  diffCounters(current.ThreadErr, prev.ThreadErr),
	}
	in.running = in.process - in.thread
	in.zombie = in.exits - in.taskClose
	return in
}

func writeProcessGlobal(api *netdataapi.API, in processInterval, usecSince int, reportErrors bool) {
	if api == nil {
		return
	}

	pluginOutputMu.Lock()
	defer pluginOutputMu.Unlock()

	api.BEGIN(processGlobalGroup, "process_thread", usecSince)
	api.SET("_do_fork", in.process)
	api.SET("sys_clone", in.thread)
	api.END()

	api.BEGIN(processGlobalGroup, "exit", usecSince)
	api.SET("do_exit", in.exits)
	api.SET("release_task", in.taskClose)
	api.END()

	api.BEGIN(processGlobalGroup, "process_status", usecSince)
	api.SET("process", in.running)
	api.SET("zombie", in.zombie)
	api.END()

	if !reportErrors {
		return
	}

	api.BEGIN(processGlobalGroup, "task_error", usecSince)
	api.SET("_do_fork", in.processErr)
	api.SET("sys_clone", in.threadErr)
	api.END()
}

// runProcessGlobalCollector is process's single collection loop: the global
// snapshot and, when apps/cgroups integration is on, the per-PID rows.
// shouldPublish is true only when process owns the shared-memory segment.
func runProcessGlobalCollector(
	api *netdataapi.API,
	handle *ProcessLegacyHandle,
	stop <-chan struct{},
	store *ebpfSharedMemoryStore,
	updateEvery int,
	shouldPublish bool,
) {
	if handle == nil || handle.Runtime == nil {
		return
	}
	if updateEvery <= 0 {
		updateEvery = processDefaultUpdateEvery
	}

	createProcessGlobalCharts(api, updateEvery, handle.ReportErrors)
	if handle.Plan.Flavor == ObjectFlavorArena {
		createArenaDropsChart(api, processGlobalModule, updateEvery)
	}

	var state processGlobalState
	collectAndPublish := func(usecSince int) {
		if handle.Plan.Flavor == ObjectFlavorArena {
			publishArenaDrops(api, processGlobalModule, handle.Runtime.ArenaDropped(), usecSince)
		}

		if snapshot, err := handle.Runtime.Snapshot(handle.MapsPerCore); err != nil {
			logPluginErr("process.snapshot", "process", "snapshot", err)
		} else {
			writeProcessGlobal(api, state.interval(snapshot), usecSince, handle.ReportErrors)
		}

		if store == nil {
			return
		}

		// Lazy open in one place so a failed first snapshot still creates the
		// segment for the other modules' rows (see runFDGlobalCollector).
		publishSharedStore := func() {
			if handle.SharedMemory == nil {
				publisher, perr := NewSharedPidMemoryPublisher(
					productionSHMName, productionSEMName, handle.PidTableSize, uint32(updateEvery))
				if perr != nil {
					logPluginErr("process.shm_open", "process", "shared memory open", perr)
				} else {
					handle.SharedMemory = publisher
				}
			}
			if handle.SharedMemory != nil {
				if perr := store.Publish(handle.SharedMemory, processSHMFlags); perr != nil {
					logPluginErr("process.publish", "process", "shared memory publish", perr)
				}
			}
		}

		apps, err := handle.Runtime.SnapshotApps(handle.MapsPerCore)
		if err != nil {
			logPluginErr("process.snapshot_apps", "process", "snapshot-apps", err)
			store.ClearProcessApps()
			if shouldPublish {
				publishSharedStore()
			}
			return
		}

		staleCandidates := store.UpdateProcessApps(apps, handle.ReportErrors, uint32(updateEvery))
		if len(staleCandidates) > 0 {
			deadPIDs := staleCandidates[:0]
			for _, pid := range staleCandidates {
				if !libbpfloader.PidIsAlive(pid) {
					deadPIDs = append(deadPIDs, pid)
				}
			}
			if len(deadPIDs) > 0 {
				if err := handle.Runtime.DeletePids(deadPIDs); err != nil {
					rateLimitedStderr("process.delete_pids",
						"ebpf-go.plugin: failed to delete %d stale PIDs from tbl_pid_stats: %v\n",
						len(deadPIDs), err)
				} else {
					store.RemoveProcessPIDs(deadPIDs)
				}
			}
		}

		if shouldPublish {
			publishSharedStore()
		}
	}

	collectAndPublish(0)
	lastCollection := time.Now()

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
