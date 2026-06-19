package main

import (
	"time"
)

// error logging reuses the shared rateLimitedStderr / logPluginErr helpers
// from error_log.go; this file only owns the collector loop.

func runDNSGlobalCollector(handle *DNSLegacyHandle, stop <-chan struct{}, updateEvery int, shm *SharedDnsMemoryPublisher) {
	if handle == nil || handle.Runtime == nil {
		return
	}

	if updateEvery <= 0 {
		updateEvery = dnsDefaultUpdateEvery
	}

	collectAndPublish := func() {
		snap, err := handle.Runtime.Snapshot()
		if err != nil {
			logPluginErr("dns.snapshot", "dns", "snapshot", err)
			return
		}

		flows, err := handle.Runtime.FlowSnapshot()
		if err != nil {
			// FlowSnapshot failure is non-fatal; publish aggregate only.
			logPluginErr("dns.flow_snapshot", "dns", "flow snapshot", err)
			flows = nil
		}

		shm.Publish(snap, flows)
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
