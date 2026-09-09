package main

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// bootClockReader returns nanoseconds since boot, reporting false when this
// platform or kernel cannot supply them.  It exists so the degrade path is
// reachable from tests without depending on the host's clocks, mirroring the
// kallsymsOpener seam in dcstat_targets.go.
type bootClockReader func() (uint64, bool)

// readBootClock is the live boot clock.  Tests swap it; production never does.
var readBootClock bootClockReader = bootNanosFromClock

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
func bootNanos() uint64 {
	// Preferred path: read the boot clock live, so the value includes any time
	// the machine spent suspended.  Nothing is cached, so there is no second
	// clock domain to drift against.
	if ns, ok := readBootClock(); ok {
		return ns
	}

	// Degraded path, only when the kernel cannot serve CLOCK_BOOTTIME.
	// /proc/uptime has centisecond resolution, so re-reading it per call would
	// return a stair-step that stalls for 10 ms at a time; it is sampled once and
	// advanced with Go's monotonic clock instead.
	//
	// That elapsed portion is CLOCK_MONOTONIC and so excludes suspend, which
	// makes the result lag the true boot clock by the suspended duration.  This
	// is safe in the only direction that matters: it can only make this
	// instance's tokens SMALLER than real boot time, and any later instance
	// re-reads the boot clock, which is always at least what this one published.
	ref := uptimeRef()
	return ref.base + uint64(time.Since(ref.start))
}

type bootClock struct {
	base  uint64    // nanoseconds since boot, sampled once
	start time.Time // monotonic reference taken together with base
}

// uptimeRef samples the coarse fallback reference once per process.  Sampling it
// per caller would let two references taken milliseconds apart disagree by up to
// 10 ms in either direction; one shared reference keeps every caller on a single
// timeline.
var uptimeRef = sync.OnceValue(func() bootClock {
	// A failed /proc/uptime read yields a process-start-relative fallback. It is
	// less safe than the boot-relative path, but remains below later boot-clock
	// tokens and avoids the catastrophic wall-clock domain jump described above.
	base, _ := bootNanosFromProcUptime()
	return bootClock{base: base, start: time.Now()}
})

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
