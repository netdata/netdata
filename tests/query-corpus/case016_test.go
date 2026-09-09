// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-016 — a child that first connects shortly before a GRACEFUL parent
// restart is entirely forgotten: host metadata persistence is asynchronous
// (a scan cycle every METADATA_HOST_CHECK_INTERVAL=5s,
// sqlite_metadata.c) and the metasync shutdown path flushes pending alerts
// and SQL statements but never runs a final host scan — the host record
// never reaches sqlite, and on the next boot the host 404s while its
// dbengine files sit orphaned on disk.
//
// Real-world: a parent restarting (e.g. updating) within seconds of a new
// child's first connection forgets that child and its streamed history.
// Children that reconnect with local retention re-register and re-replicate
// (self-healing); ephemeral or no-retention children lose the data
// permanently — the same healing asymmetry as CASE-015.
//
// FIXED by #23120 (merged 2026-07-16): the metasync shutdown path now
// runs a final host scan, so the fresh host persists regardless of scan
// phase. The bounded post-restart attempt prevents unbounded timing drift,
// but its 10-second budget does not distinguish shutdown persistence from
// the ordinary 5-second metadata scan.
package corpus

import (
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/fixture"
)

const (
	c016ScanPhaseBudget = 10 * time.Second
	// This is a sub-budget: retention must leave time for a fully reaped
	// shutdown inside the whole-process phase budget above.
	c016RetentionBudget = time.Second
	c016MaxAttempts     = 5
)

func c016DurationsWithinBudget(monotonic, wall, budget time.Duration) bool {
	return budget > 0 &&
		monotonic >= 0 && monotonic < budget &&
		wall >= 0 && wall < budget
}

func c016PhaseWithinBudget(start, now time.Time, budget time.Duration) bool {
	if start.IsZero() {
		return false
	}
	return c016DurationsWithinBudget(
		now.Sub(start),
		time.Duration(now.UnixNano()-start.UnixNano()),
		budget)
}

func TestCase016FreshHostForgottenOnRestart(t *testing.T) {
	aged := fixture.Series("fixture.c016aged", "fixture.c016aged", fixture.T0, 60, 1, modVal, notAnom)
	pushLiveBurst(t, "c016-aged", guid(40), aged)
	settleAndVerify(t, "c016-aged", aged)

	// Persist the control before establishing the first measured phase.
	time.Sleep(8 * time.Second)
	if err := td.Restart(); err != nil {
		t.Fatalf("establish CASE-016 scan phase: %v", err)
	}

	requireAged := func() {
		t.Helper()
		if _, err := td.WaitRetention(
			"c016-aged", aged.Context, aged.FirstT(), aged.LastT(), 10*time.Second); err != nil {
			t.Fatalf("control failed: aged host lost across restart: %v", err)
		}
	}
	requireAged()
	if err := td.Restart(); err != nil {
		t.Fatalf("start first measured CASE-016 phase: %v", err)
	}

	validAttempt := 0
	contractHeld := false
	for attempt := 1; attempt <= c016MaxAttempts; attempt++ {
		context := fmt.Sprintf("fixture.c016fresh%d", attempt)
		hostname := fmt.Sprintf("c016-fresh-%d", attempt)
		fresh := fixture.Series(context, context, fixture.T0, 60, 1, modVal, notAnom)
		phaseStart := td.LaunchStartedAt
		invalidReason := ""

		if !c016PhaseWithinBudget(phaseStart, time.Now(), c016ScanPhaseBudget) {
			invalidReason = "daemon readiness exhausted the scan-phase budget"
		} else {
			pushLiveBurst(t, hostname, guid(190+attempt), fresh)
			if _, err := td.WaitRetention(
				hostname, fresh.Context, fresh.FirstT(), fresh.LastT(), c016RetentionBudget); err != nil {
				invalidReason = fmt.Sprintf("fresh retention did not settle inside %s: %v",
					c016RetentionBudget, err)
			}
		}

		if err := td.Stop(); err != nil {
			t.Fatalf("stop CASE-016 attempt %d: %v", attempt, err)
		}
		phaseEnd := time.Now()
		monotonicElapsed := phaseEnd.Sub(phaseStart)
		wallElapsed := time.Duration(phaseEnd.UnixNano() - phaseStart.UnixNano())
		if !c016DurationsWithinBudget(
			monotonicElapsed, wallElapsed, c016ScanPhaseBudget) {
			invalidReason = fmt.Sprintf(
				"old process lifetime %s monotonic/%s wall crossed the %s scan-phase budget",
				monotonicElapsed, wallElapsed, c016ScanPhaseBudget)
		}
		if err := td.Restart(); err != nil {
			t.Fatalf("restart after CASE-016 attempt %d: %v", attempt, err)
		}
		requireAged()

		if invalidReason != "" {
			t.Logf("attempt %d invalid: %s", attempt, invalidReason)
			if err := td.Restart(); err != nil {
				t.Fatalf("start measured CASE-016 phase after attempt %d: %v", attempt, err)
			}
			continue
		}

		validAttempt = attempt
		trackContract(t, "CASE-016/fresh-host-forgotten-on-restart")
		t.Logf(
			"valid attempt %d: fresh retention settled before shutdown; old process fully reaped after %s monotonic/%s wall",
			attempt, monotonicElapsed, wallElapsed)
		if _, err := td.WaitRetention(
			hostname, fresh.Context, fresh.FirstT(), fresh.LastT(), 10*time.Second); err != nil {
			t.Logf("valid attempt %d: fresh host lost: %v", attempt, err)
			break
		}
		settleAndVerify(t, hostname, fresh)
		contractHeld = true
		t.Logf("valid attempt %d: fresh host survived", attempt)
		break
	}

	if validAttempt == 0 {
		t.Fatalf(
			"CASE-016 phase control produced no valid attempt in %d tries; no contract verdict is supported",
			c016MaxAttempts)
	}

	assertContract(t, "CASE-016/fresh-host-forgotten-on-restart", contractHeld)
}
