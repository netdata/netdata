package main

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
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
		units:     "pages/s",
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
		algorithm: "absolute",
	},
	{
		id:        "cachestat_misses",
		title:     "Files out of page cache",
		units:     "misses/s",
		context:   "mem.cachestat_misses",
		order:     21103,
		dimension: "miss",
		algorithm: "absolute",
	},
}

var cachestatGlobalChartsOnce sync.Once
var cachestatStdoutMutex sync.Mutex

func (s *cachestatGlobalState) Update(current cachestatGlobalCounters) (cachestatGlobalPublish, bool) {
	mpa := diffCounters(current.MarkPageAccessed, s.prev.MarkPageAccessed)
	mbd := diffCounters(current.MarkBufferDirty, s.prev.MarkBufferDirty)
	apcl := diffCounters(current.AddToPageCacheLru, s.prev.AddToPageCacheLru)
	apd := diffCounters(current.AccountPageDirtied, s.prev.AccountPageDirtied)

	publish := cachestatGlobalPublish{
		Dirty: mbd,
	}

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

	publish.Hit = hits
	publish.Miss = misses
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

func (p cachestatGlobalPublish) write(api *netdataapi.API, msSince int) {
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
		api.BEGIN(cachestatGlobalGroup, item.chart, msSince)
		api.SET(item.dim, item.value)
		api.END()
	}
}

func runCachestatGlobalCollector(api *netdataapi.API, handle *CachestatLegacyHandle, stop <-chan struct{}, updateEvery int) {
	if handle == nil || handle.Runtime == nil {
		return
	}

	if updateEvery <= 0 {
		updateEvery = cachestatDefaultUpdateEvery
	}

	createCachestatGlobalCharts(api, updateEvery)

	state := &cachestatGlobalState{}
	lastCollection := time.Now()
	collectAndPublish := func(msSince int) {
		snapshot, err := handle.Runtime.Snapshot(true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ebpf-go.plugin: cachestat snapshot failed: %v\n", err)
			return
		}

		publish, ok := state.Update(cachestatGlobalCounters{
			MarkPageAccessed:   snapshot.MarkPageAccessed,
			MarkBufferDirty:    snapshot.MarkBufferDirty,
			AddToPageCacheLru:  snapshot.AddToPageCacheLru,
			AccountPageDirtied: snapshot.AccountPageDirtied,
		})
		if !ok {
			return
		}

		publish.write(api, msSince)
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
		msSince := int(now.Sub(lastCollection).Milliseconds())
		if msSince < 0 {
			msSince = 0
		}
		lastCollection = now
		collectAndPublish(msSince)
	}
}

func runCachestatPlugin(handle *CachestatLegacyHandle, updateEveryArg int) {
	if handle == nil || handle.Runtime == nil {
		return
	}

	updateEvery := cachestatDefaultUpdateEvery
	if handle.UpdateEvery > 0 {
		updateEvery = handle.UpdateEvery
	}
	if updateEveryArg > 0 {
		updateEvery = updateEveryArg
	}
	api := netdataapi.New(os.Stdout)

	store := NewCachestatSharedMemoryStore()
	service := NewSharedSnapshotService(
		"/var/run/netdata",
		defaultSharedSnapshotServiceName,
		defaultSharedSnapshotServerConfig(),
		nil,
	)

	if service == nil {
		return
	}

	stop := make(chan struct{})
	doneService := make(chan struct{})
	go func() {
		defer close(doneService)
		_ = service.Run()
	}()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runCachestatSharedMemoryCollector(handle, stop, store)
	}()

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		signal.Stop(sigCh)

		close(stop)
		service.Stop()
	}()

	runCachestatGlobalCollector(api, handle, stop, updateEvery)

	wg.Wait()
	handle.Close()
	<-doneService
}
