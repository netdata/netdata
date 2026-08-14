package main

import (
	"testing"
	"time"
)

// TestBootSourcesShareOneDomain pins the rule that every reference source stays
// boot-relative.
//
// The earlier implementation fell back to time.Now().UnixNano() when /proc/uptime
// could not be read. That is a different domain by five orders of magnitude: one
// instance taking the fallback would publish tokens near 1.7e18 instead of ~1e13,
// push every consumer watermark decades ahead, and freeze the charts permanently
// for every later instance — strictly worse than having no reference at all.
//
// This is the "fallback followed by a successful read" case: whichever source a
// given process lands on, the values must be interchangeable.
func TestBootSourcesShareOneDomain(t *testing.T) {
	clockNs, clockOK := bootNanosFromClock()
	uptimeNs, uptimeOK := bootNanosFromProcUptime()

	if !clockOK && !uptimeOK {
		t.Skip("no boot-relative source available on this host")
	}
	if !clockOK || !uptimeOK {
		t.Skipf("only one source available (clock=%v uptime=%v); cannot compare domains", clockOK, uptimeOK)
	}

	// Sampled microseconds apart, so anything beyond a couple of seconds means the
	// two sources are not measuring the same thing.
	const tolerance = uint64(2 * time.Second)
	diff := clockNs - uptimeNs
	if uptimeNs > clockNs {
		diff = uptimeNs - clockNs
	}
	if diff > tolerance {
		t.Fatalf("boot sources disagree by %v (clock=%d uptime=%d): they must share one domain",
			time.Duration(diff), clockNs, uptimeNs)
	}

	// A wall-clock value would be ~1.7e18. Nothing boot-relative reaches that
	// unless the machine has been up for ~54 years.
	const wallClockFloor = uint64(1_000_000_000_000_000_000)
	if clockNs >= wallClockFloor || uptimeNs >= wallClockFloor {
		t.Fatalf("a source returned a wall-clock magnitude value (clock=%d uptime=%d)", clockNs, uptimeNs)
	}
}

// TestReadBootNanosOrdersReferencesWithinOneTick pins that two references taken
// inside the same /proc/uptime tick are still strictly ordered.
//
// /proc/uptime only has centisecond resolution. If it were the primary source,
// two process lifetimes starting within the same 10 ms tick would read an
// identical base, and the one that started later — with a smaller monotonic
// offset — could publish a token BELOW the watermark the earlier one left. The
// nanosecond-resolution clock removes that window.
func TestReadBootNanosOrdersReferencesWithinOneTick(t *testing.T) {
	if _, ok := bootNanosFromClock(); !ok {
		t.Skip("nanosecond boot clock unavailable; /proc/uptime resolution is expected to be coarse")
	}

	const samples = 200
	var distinct int
	prev := readBootNanos()
	if prev == 0 {
		t.Fatal("readBootNanos returned 0 while a boot clock is available")
	}

	deadline := time.Now().Add(5 * time.Millisecond)
	for i := range samples {
		got := readBootNanos()
		if got < prev {
			t.Fatalf("sample %d went backwards: %d after %d", i, got, prev)
		}
		if got > prev {
			distinct++
		}
		prev = got
		if time.Now().After(deadline) {
			break
		}
	}

	// Within a single centisecond tick the coarse source would report one value
	// for every sample. Seeing the reference move proves sub-tick resolution.
	if distinct == 0 {
		t.Fatal("readBootNanos never advanced across samples taken inside one tick: " +
			"the reference is quantized, so restarts within a tick can invert")
	}
}

// TestBootNanosIsMonotonic pins the process-wide invariant the token generator
// depends on: the clock never goes backwards, so a store created later always
// sees values at least as large as one created earlier.
func TestBootNanosIsMonotonic(t *testing.T) {
	early := bootNanos()
	if early == 0 {
		t.Fatal("bootNanos reported 0; no boot-relative source worked")
	}

	for i := range 100 {
		got := bootNanos()
		if got < early {
			t.Fatalf("call %d reported %d, an earlier call reported %d: must never go backwards",
				i, got, early)
		}
		early = got
	}
}
