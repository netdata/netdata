package main

import (
	"fmt"
	"os"
	"sync"
	"time"
)

const dnsErrorLogInterval = 60 * time.Second

var (
	dnsErrorMu      sync.Mutex
	dnsErrorLastLog = map[string]time.Time{}
)

func dnsRateLimitedStderr(site, msg string) {
	now := time.Now()
	dnsErrorMu.Lock()
	if last, ok := dnsErrorLastLog[site]; ok && now.Sub(last) < dnsErrorLogInterval {
		dnsErrorMu.Unlock()
		return
	}
	dnsErrorLastLog[site] = now
	dnsErrorMu.Unlock()
	fmt.Fprint(os.Stderr, msg)
}

func dnsLogErr(site, what string, err error) {
	dnsRateLimitedStderr(site, fmt.Sprintf("ebpf-go.plugin: dns %s failed: %v\n", what, err))
}

func runDNSGlobalCollector(handle *DNSLegacyHandle, stop <-chan struct{}, updateEvery int, fnStore *dnsFunctionStore) {
	if handle == nil || handle.Runtime == nil {
		return
	}

	if updateEvery <= 0 {
		updateEvery = dnsDefaultUpdateEvery
	}

	collectAndPublish := func() {
		snap, err := handle.Runtime.Snapshot()
		if err != nil {
			dnsLogErr("dns.snapshot", "snapshot", err)
			return
		}
		fnStore.update(snap)
	}

	collectAndPublish()

	ticker := time.NewTicker(time.Duration(updateEvery) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		}

		collectAndPublish()
	}
}
