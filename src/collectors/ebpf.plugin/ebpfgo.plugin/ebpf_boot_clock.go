package main

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// bootNanos reports nanoseconds since boot, the same domain the BPF `ct` field
// carries on the object flavors that stamp it at all.
//
// The shared-memory store draws its synthetic freshness tokens from this clock
// rather than from a plain counter because the consumers' watermarks outlive
// this process.  cgroups.plugin is compiled into the netdata daemon and its
// per-cgroup watermark (cg->dcstat.ct) only ever moves forward — there is no
// regression reset, deliberately, so that a PID leaving a cgroup cannot replay
// rows.  A counter restarting at 1 after an ebpf-go.plugin restart would
// therefore sit below the watermark the daemon still holds, and every PID would
// fail the `ct > prev_ct` gate for as many cycles as the previous instance ran,
// freezing the cgroup charts for hours.
//
// A boot-relative clock has the property the BPF stamp had: it keeps rising
// across plugin restarts, and resets only on reboot — which also restarts the
// daemon and zeroes the watermark.
//
// EVERY source here MUST stay in the boot-relative domain.  Falling back to the
// wall clock would be far worse than having no reference at all: one instance
// that took the fallback would publish tokens around 1.7e18 instead of ~1e13,
// pushing every consumer watermark decades into the future and freezing the
// charts permanently for every later instance, not just its own.
//
// The reference is sampled ONCE per process.  The /proc/uptime fallback has only
// centisecond resolution, so sampling it per caller would let two references
// taken milliseconds apart disagree by up to 10 ms in either direction; one
// shared reference plus Go's monotonic clock keeps every caller on a single
// timeline.
func bootNanos() uint64 {
	ref := bootClockRef()
	return ref.base + uint64(time.Since(ref.start))
}

type bootClock struct {
	base  uint64    // nanoseconds since boot, sampled once
	start time.Time // monotonic reference taken together with base
}

var bootClockRef = sync.OnceValue(func() bootClock {
	return bootClock{base: readBootNanos(), start: time.Now()}
})

// readBootNanos returns nanoseconds since boot, preferring the nanosecond-
// resolution clock and falling back to /proc/uptime, which is the same
// boot-relative domain at centisecond resolution (and the source libnetdata's
// boottime.c uses).
//
// If neither is available it returns 0.  That degrades this instance — its own
// tokens start below any watermark a previous instance left — but it stays in
// the boot domain, so it cannot poison the watermark for instances that come
// after it.
func readBootNanos() uint64 {
	if ns, ok := bootNanosFromClock(); ok {
		return ns
	}
	if ns, ok := bootNanosFromProcUptime(); ok {
		return ns
	}
	return 0
}

// bootNanosFromProcUptime parses the first field of /proc/uptime, seconds since
// boot including time spent suspended.
func bootNanosFromProcUptime() (uint64, bool) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, false
	}

	field, _, _ := strings.Cut(strings.TrimSpace(string(data)), " ")
	seconds, err := strconv.ParseFloat(field, 64)
	if err != nil || seconds < 0 {
		return 0, false
	}

	return uint64(seconds * float64(time.Second)), true
}
