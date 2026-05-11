package main

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const (
	cachestatGlobalGroup     = "mem"
	cachestatGlobalFamily    = "page_cache"
	cachestatGlobalModule    = "cachestat"
	cachestatGlobalUpdateSec = 10
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
	id      string
	title   string
	units   string
	context string
	order   int
	dim     string
	metric  string
}

var cachestatGlobalCharts = []cachestatGlobalChart{
	{
		id:      "cachestat_ratio",
		title:   "Hit ratio",
		units:   "percentage",
		context: "mem.cachestat_ratio",
		order:   21100,
		dim:     "percentage",
	},
	{
		id:      "cachestat_dirties",
		title:   "Number of dirty pages",
		units:   "pages/s",
		context: "mem.cachestat_dirties",
		order:   21101,
		dim:     "pages",
	},
	{
		id:      "cachestat_hits",
		title:   "Number of accessed files",
		units:   "hits/s",
		context: "mem.cachestat_hits",
		order:   21102,
		dim:     "hits",
	},
	{
		id:      "cachestat_misses",
		title:   "Files out of page cache",
		units:   "misses/s",
		context: "mem.cachestat_misses",
		order:   21103,
		dim:     "misses",
	},
}

var cachestatGlobalChartsOnce sync.Once

func (s *cachestatGlobalState) Update(current cachestatGlobalCounters) (cachestatGlobalPublish, bool) {
	if !s.initialized {
		s.prev = current
		s.initialized = true
		return cachestatGlobalPublish{}, false
	}

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

	return publish, true
}

func diffCounters(current, previous uint64) int64 {
	if current < previous {
		return 0
	}

	return int64(current - previous)
}

func createCachestatGlobalCharts(updateEvery int) {
	cachestatGlobalChartsOnce.Do(func() {
		for _, chart := range cachestatGlobalCharts {
			fmt.Printf(
				"CHART %s.%s '' '%s' '%s' '%s' '%s' '%s' %d %d '' 'ebpf.plugin' '%s'\n",
				cachestatGlobalGroup,
				chart.id,
				chart.title,
				chart.units,
				cachestatGlobalFamily,
				chart.context,
				"line",
				chart.order,
				updateEvery,
				cachestatGlobalModule,
			)
			fmt.Printf("DIMENSION %s %s %s 1 1\n", chart.dim, chart.dim, "absolute")
		}
	})
}

func (p cachestatGlobalPublish) write() {
	for _, item := range []struct {
		chart string
		dim   string
		value int64
	}{
		{chart: "cachestat_ratio", dim: "percentage", value: p.Ratio},
		{chart: "cachestat_dirties", dim: "pages", value: p.Dirty},
		{chart: "cachestat_hits", dim: "hits", value: p.Hit},
		{chart: "cachestat_misses", dim: "misses", value: p.Miss},
	} {
		fmt.Printf("BEGIN %s.%s\n", cachestatGlobalGroup, item.chart)
		fmt.Printf("SET %s = %d\n", item.dim, item.value)
		fmt.Println("END")
	}
}

func runCachestatGlobalCollector(handle *CachestatLegacyHandle, stop <-chan struct{}) {
	if handle == nil || handle.Runtime == nil {
		return
	}

	createCachestatGlobalCharts(cachestatGlobalUpdateSec)

	ticker := time.NewTicker(time.Duration(cachestatGlobalUpdateSec) * time.Second)
	defer ticker.Stop()

	state := &cachestatGlobalState{}
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		}

		snapshot, err := handle.Runtime.Snapshot(true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ebpf-go.plugin: cachestat snapshot failed: %v\n", err)
			continue
		}

		publish, ok := state.Update(cachestatGlobalCounters{
			MarkPageAccessed:   snapshot.MarkPageAccessed,
			MarkBufferDirty:    snapshot.MarkBufferDirty,
			AddToPageCacheLru:  snapshot.AddToPageCacheLru,
			AccountPageDirtied: snapshot.AccountPageDirtied,
		})
		if !ok {
			continue
		}

		publish.write()
	}
}

func runCachestatPlugin(handle *CachestatLegacyHandle) {
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

	doneCollector := make(chan struct{})
	if handle != nil && handle.Runtime != nil {
		go func() {
			defer close(doneCollector)
			runCachestatSharedMemoryCollector(handle, stop, store)
		}()
	} else {
		close(doneCollector)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	signal.Stop(sigCh)

	close(stop)
	service.Stop()
	<-doneCollector
	if handle != nil {
		handle.Close()
	}
	<-doneService
}
