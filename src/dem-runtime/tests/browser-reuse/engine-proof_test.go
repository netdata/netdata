//go:build linux

// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

func TestDedicatedArrivalPlan(t *testing.T) {
	options := workloadOptions{window: 15 * time.Minute, journeyEvery: 5 * time.Minute, auditEvery: 15 * time.Minute}
	plan := planWorkload(300, 100, options)
	if len(plan) != 1000 {
		t.Fatalf("expected 900 journeys and 100 audits, got %d arrivals", len(plan))
	}
	jobs := make(map[string][]plannedAttempt)
	counts := make(map[synthetic.Kind]int)
	var previous time.Duration
	for _, attempt := range plan {
		if attempt.due < previous || attempt.due < 0 || attempt.due >= options.window {
			t.Fatalf("arrival outside ordered half-open window: %+v", attempt)
		}
		previous = attempt.due
		jobs[attempt.name] = append(jobs[attempt.name], attempt)
		counts[attempt.kind]++
	}
	if len(jobs) != 400 || counts[synthetic.Journey] != 900 || counts[synthetic.Lighthouse] != 100 {
		t.Fatalf("unexpected identities or mix: %d jobs, %v", len(jobs), counts)
	}
	for name, attempts := range jobs {
		want := 1
		if attempts[0].kind == synthetic.Journey {
			want = 3
		}
		if len(attempts) != want {
			t.Fatalf("%s: expected %d occurrences, got %d", name, want, len(attempts))
		}
		for i := 1; i < len(attempts); i++ {
			if attempts[i].due-attempts[i-1].due != 5*time.Minute {
				t.Fatalf("%s: journey cadence drifted", name)
			}
		}
	}
}

func TestBarrierPlanAndCancelledWait(t *testing.T) {
	plan := planWorkload(120, 0, workloadOptions{journeyEvery: time.Minute, auditEvery: time.Minute})
	if len(plan) != 120 {
		t.Fatalf("expected 120 arrivals, got %d", len(plan))
	}
	for _, attempt := range plan {
		if attempt.due != 0 || attempt.kind != synthetic.Journey {
			t.Fatalf("unexpected barrier attempt: %+v", attempt)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitUntil(ctx, time.Now().Add(time.Hour)); err != context.Canceled {
		t.Fatalf("expected cancellation without waiting for deadline, got %v", err)
	}
}
