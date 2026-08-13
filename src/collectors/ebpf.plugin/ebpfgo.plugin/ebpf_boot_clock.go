package main

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// bootNanos reports nanoseconds since boot, reproducing the quantity
// bpf_ktime_get_ns() stamps into the BPF `ct` field on the object flavors that
// set it at all.
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
// The reference is sampled ONCE per process.  /proc/uptime only has centisecond
// resolution, so sampling it per caller would let two references taken
// milliseconds apart disagree by up to 10 ms in either direction; one shared
// reference plus Go's monotonic clock keeps every caller on a single timeline.
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

// readBootNanos reads /proc/uptime, the same source libnetdata's boottime.c uses.
//
// On failure it falls back to the wall clock.  That is not immune to clock steps,
// but it still keeps a restarted plugin's tokens above the previous instance's,
// which is the property the consumers depend on.
func readBootNanos() uint64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return uint64(time.Now().UnixNano())
	}

	field, _, _ := strings.Cut(strings.TrimSpace(string(data)), " ")
	seconds, err := strconv.ParseFloat(field, 64)
	if err != nil || seconds < 0 {
		return uint64(time.Now().UnixNano())
	}

	return uint64(seconds * float64(time.Second))
}
