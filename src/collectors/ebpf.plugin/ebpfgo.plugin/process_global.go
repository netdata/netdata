package main

import (
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

const (
	processGlobalGroup  = "system"
	processGlobalFamily = "processes"
	processGlobalModule = "process"
	processGlobalPlugin = "ebpf-go.plugin"
)

type processGlobalCounters struct {
	Exits     uint64
	TaskClose uint64
	Forks     uint64
	Clones    uint64
	Errors    uint64
}

type processGlobalPublish struct {
	Exits     int64
	TaskClose int64
	Forks     int64
	Clones    int64
	Errors    int64
}

type processGlobalChart struct {
	id        string
	title     string
	units     string
	context   string
	order     int
	dimension string
	algorithm string
}

var processGlobalCharts = []processGlobalChart{
	{
		id:        "process_thread",
		title:     "Start process",
		units:     "calls/s",
		context:   "system.process_thread",
		order:     21002,
		dimension: "calls",
		algorithm: "incremental",
	},
	{
		id:        "exit",
		title:     "Exit process",
		units:     "calls/s",
		context:   "system.exit",
		order:     21003,
		dimension: "calls",
		algorithm: "incremental",
	},
	{
		id:        "task_close",
		title:     "Tasks released",
		units:     "calls/s",
		context:   "system.task_close",
		order:     21004,
		dimension: "calls",
		algorithm: "incremental",
	},
	{
		id:        "task_error",
		title:     "Errors in process/thread creation",
		units:     "calls/s",
		context:   "system.task_error",
		order:     21005,
		dimension: "errors",
		algorithm: "incremental",
	},
}

var processGlobalChartsOnce sync.Once

func (s *processGlobalState) Update(current processGlobalCounters) processGlobalPublish {
	p := processGlobalPublish{
		Exits:     diffCounters(current.Exits, s.prev.Exits),
		TaskClose: diffCounters(current.TaskClose, s.prev.TaskClose),
		Forks:     diffCounters(current.Forks, s.prev.Forks),
		Clones:    diffCounters(current.Clones, s.prev.Clones),
		Errors:    diffCounters(current.Errors, s.prev.Errors),
	}
	s.prev = current
	return p
}

func createProcessGlobalCharts(api *netdataapi.API, updateEvery int) {
	processGlobalChartsOnce.Do(func() {
		pluginOutputMu.Lock()
		defer pluginOutputMu.Unlock()
		if api != nil {
			api.HOST("")
		}
		for _, chart := range processGlobalCharts {
			if api == nil {
				continue
			}
			api.CHART(netdataapi.ChartOpts{TypeID: processGlobalGroup, ID: chart.id, Title: chart.title,
				Units: chart.units, Family: processGlobalFamily, Context: chart.context, ChartType: "line",
				Priority: chart.order, UpdateEvery: updateEvery, Plugin: processGlobalPlugin, Module: processGlobalModule})
			api.DIMENSION(netdataapi.DimensionOpts{ID: chart.dimension, Name: chart.dimension, Algorithm: chart.algorithm, Multiplier: 1, Divisor: 1})
		}
	})
}

func (p processGlobalPublish) write(api *netdataapi.API, usecSince int) {
	if api == nil {
		return
	}
	pluginOutputMu.Lock()
	defer pluginOutputMu.Unlock()
	for _, item := range []struct {
		chart, dim string
		value      int64
	}{
		{"process_thread", "calls", p.Forks + p.Clones},
		{"exit", "calls", p.Exits},
		{"task_close", "calls", p.TaskClose},
		{"task_error", "errors", p.Errors},
	} {
		api.BEGIN(processGlobalGroup, item.chart, usecSince)
		api.SET(item.dim, item.value)
		api.END()
	}
}

func runProcessGlobalCollector(api *netdataapi.API, handle *ProcessLegacyHandle, stop <-chan struct{}, store *ebpfSharedMemoryStore, updateEvery int, shouldPublish bool) {
	if handle == nil || handle.Runtime == nil {
		return
	}
	if updateEvery <= 0 {
		updateEvery = processDefaultUpdateEvery
	}
	createProcessGlobalCharts(api, updateEvery)
	state := processGlobalState{}
	last := time.Now()
	collect := func(usecSince int) {
		if snapshot, err := handle.Runtime.Snapshot(handle.MapsPerCore); err != nil {
			logPluginErr("process.snapshot", "process", "snapshot", err)
		} else {
			p := state.Update(processGlobalCounters{Exits: snapshot.Exits, TaskClose: snapshot.TaskClose, Forks: snapshot.Forks, Clones: snapshot.Clones, Errors: snapshot.Errors})
			p.write(api, usecSince)
		}
		if store == nil || !handle.AppsEnabled && !handle.CgroupsEnabled {
			return
		}
		apps, err := handle.Runtime.SnapshotApps(handle.MapsPerCore)
		if err != nil {
			logPluginErr("process.snapshot_apps", "process", "snapshot-apps", err)
			store.ClearProcessApps()
		} else {
			stale := store.UpdateAppsProcess(apps)
			if len(stale) > 0 {
				dead := stale[:0]
				for _, pid := range stale {
					if !libbpfloader.PidIsAlive(pid) {
						dead = append(dead, pid)
					}
				}
				if len(dead) > 0 {
					if derr := handle.Runtime.DeletePids(dead); derr != nil {
						logPluginErr("process.delete_pids", "process", "delete stale PIDs", derr)
					} else {
						store.RemoveProcessPIDs(dead)
					}
				}
			}
		}
		if shouldPublish {
			if handle.SharedMemory == nil {
				publisher, perr := NewSharedPidMemoryPublisher(productionSHMName, productionSEMName, handle.PidTableSize, uint32(updateEvery))
				if perr != nil {
					logPluginErr("process.shm_open", "process", "shared memory open", perr)
				} else {
					handle.SharedMemory = publisher
				}
			}
			if handle.SharedMemory != nil {
				if perr := store.Publish(handle.SharedMemory, ebpfgoSHMFlagProcess); perr != nil {
					logPluginErr("process.publish", "process", "shared memory publish", perr)
				}
			}
		}
	}
	collect(0)
	last = time.Now()
	ticker := time.NewTicker(time.Duration(updateEvery) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			now := time.Now()
			collect(max(int(now.Sub(last).Microseconds()), 0))
			last = now
		}
	}
}

type processGlobalState struct {
	prev             processGlobalCounters
	cumExits         int64
	cumCloses        int64
	cumForks         int64
	cumClones        int64
	lastPublish      processGlobalPublish
	lastUpdate       time.Time
	hasPublishedData bool
}

// netdataProcess represents a single per-PID process snapshot
type netdataProcess struct {
	Exits     uint32
	TaskClose uint32
	Forks     uint32
	Clones    uint32
	Errors    uint32
}

// netdataPublishProcess is the published (incremental) data for a per-PID process
type netdataPublishProcess struct {
	Ct        uint64
	Current   netdataProcess
	Prev      netdataProcess
	Exits     int64
	TaskClose int64
	Forks     int64
	Clones    int64
	Errors    int64
}
