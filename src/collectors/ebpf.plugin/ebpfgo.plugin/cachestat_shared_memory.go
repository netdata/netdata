package main

import (
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

type cachestatSharedMemoryStore struct {
	mu      sync.RWMutex
	entries []ebpfPidStat
	prev    map[uint32]netdataCachestat
}

func NewCachestatSharedMemoryStore() *cachestatSharedMemoryStore {
	return &cachestatSharedMemoryStore{
		prev: make(map[uint32]netdataCachestat),
	}
}

func (s *cachestatSharedMemoryStore) Replace(snapshot []ebpfPidStat, prev map[uint32]netdataCachestat) {
	s.mu.Lock()
	defer s.mu.Unlock()

	copied := make([]ebpfPidStat, len(snapshot))
	copy(copied, snapshot)
	s.entries = copied

	nextPrev := make(map[uint32]netdataCachestat, len(prev))
	for pid, counters := range prev {
		nextPrev[pid] = counters
	}
	s.prev = nextPrev
}

func (s *cachestatSharedMemoryStore) Snapshot() []ebpfPidStat {
	s.mu.RLock()
	defer s.mu.RUnlock()

	copied := make([]ebpfPidStat, len(s.entries))
	copy(copied, s.entries)
	sort.Slice(copied, func(i, j int) bool {
		return copied[i].pid < copied[j].pid
	})

	return copied
}

func buildCachestatPublish(current, previous netdataCachestat, ct uint64, hasPrevious bool) netdataPublishCachestat {
	publish := netdataPublishCachestat{
		Ct:      ct,
		Current: current,
		Prev:    previous,
	}

	if !hasPrevious {
		return publish
	}

	mpa := diffCounters(uint64(current.MarkPageAccessed), uint64(previous.MarkPageAccessed))
	mbd := diffCounters(uint64(current.MarkBufferDirty), uint64(previous.MarkBufferDirty))
	apcl := diffCounters(uint64(current.AddToPageCacheLru), uint64(previous.AddToPageCacheLru))
	apd := diffCounters(uint64(current.AccountPageDirtied), uint64(previous.AccountPageDirtied))

	publish.Dirty = mbd

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
	return publish
}

func copyCommFromSnapshot(dst *[EBPF_MAX_COMPARE_NAME + 1]byte, src [libbpfloader.CachestatAppCommLen]byte) {
	copy(dst[:], src[:])
}

func buildCachestatPidStat(app libbpfloader.CachestatAppSnapshot, previous netdataCachestat, hasPrevious bool) (ebpfPidStat, netdataCachestat) {
	current := netdataCachestat{
		AddToPageCacheLru:  app.AddToPageCacheLru,
		MarkPageAccessed:   app.MarkPageAccessed,
		AccountPageDirtied: app.AccountPageDirtied,
		MarkBufferDirty:    app.MarkBufferDirty,
	}

	var comm [EBPF_MAX_COMPARE_NAME + 1]byte
	copyCommFromSnapshot(&comm, app.Comm)

	return ebpfPidStat{
		pid:       app.Pid,
		comm:      comm,
		ppid:      app.Ppid,
		cachestat: buildCachestatPublish(current, previous, app.Ct, hasPrevious),
	}, current
}

func (s *cachestatSharedMemoryStore) UpdateApps(apps []libbpfloader.CachestatAppSnapshot) {
	nextEntries := make([]ebpfPidStat, 0, len(apps))
	nextPrev := make(map[uint32]netdataCachestat, len(apps))

	for _, app := range apps {
		previous, hasPrevious := s.prev[app.Pid]
		stat, current := buildCachestatPidStat(app, previous, hasPrevious)
		nextEntries = append(nextEntries, stat)
		nextPrev[app.Pid] = current
	}

	s.Replace(nextEntries, nextPrev)
}

func runCachestatSharedMemoryCollector(handle *CachestatLegacyHandle, stop <-chan struct{}, store *cachestatSharedMemoryStore, updateEvery int) {
	if handle == nil || handle.Runtime == nil || store == nil {
		return
	}

	if updateEvery <= 0 {
		updateEvery = cachestatDefaultUpdateEvery
	}

	ticker := time.NewTicker(time.Duration(updateEvery) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		}

		apps, err := handle.Runtime.SnapshotApps(true)
		if err != nil {
			continue
		}

		store.UpdateApps(apps)
		if handle.SharedMemory != nil {
			if err := handle.SharedMemory.Publish(store.Snapshot()); err != nil {
				fmt.Fprintf(os.Stderr, "ebpf-go.plugin: cachestat shared memory publish failed: %v\n", err)
			}
		}
	}
}
