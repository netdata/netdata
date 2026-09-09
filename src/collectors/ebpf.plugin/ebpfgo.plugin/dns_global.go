package main

import (
	"time"
)

// error logging reuses the shared rateLimitedStderr / logPluginErr helpers
// from error_log.go; this file only owns the collector loop.

func runDNSGlobalCollector(handle *DNSLegacyHandle, stop <-chan struct{}, updateEvery int) {
	if handle == nil || handle.Runtime == nil {
		return
	}

	if updateEvery <= 0 {
		updateEvery = dnsDefaultUpdateEvery
	}

	var shm *SharedDnsMemoryPublisher
	defer func() {
		if shm != nil {
			shm.Close()
		}
	}()

	collectAndPublish := func() {
		flows, err := handle.Runtime.FlowSnapshot()
		if err != nil {
			logPluginErr("dns.flow_snapshot", "dns", "flow snapshot", err)
			return
		}

		// Lazy SHM open: retry every cycle so a transient open failure at
		// startup self-heals within one collection interval.
		if shm == nil {
			publisher, perr := NewSharedDnsMemoryPublisher(uint32(updateEvery))
			if perr != nil {
				logPluginErr("dns.shm_open", "dns", "shared memory open", perr)
			} else {
				shm = publisher
			}
		}
		if shm != nil {
			shm.Publish(flows)
		}
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
