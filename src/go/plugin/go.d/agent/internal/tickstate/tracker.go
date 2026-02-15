// SPDX-License-Identifier: GPL-3.0-or-later

package tickstate

import (
	"sync"
	"time"
)

// SkipSnapshot is the skip-state view returned from MarkSkipped().
type SkipSnapshot struct {
	Count      int
	RunStarted time.Time
}

// ResumeSnapshot is the resume-state view returned from MarkRunStart().
type ResumeSnapshot struct {
	Skipped    int
	RunStarted time.Time
	RunStopped time.Time
}

// SkipTracker tracks skipped scheduler ticks and run timing in a thread-safe way.
type SkipTracker struct {
	mu         sync.Mutex
	skipped    int
	runStarted time.Time
	runStopped time.Time
}

// MarkSkipped records one dropped tick and returns current skip state.
func (t *SkipTracker) MarkSkipped() SkipSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.skipped++
	return SkipSnapshot{
		Count:      t.skipped,
		RunStarted: t.runStarted,
	}
}

// MarkRunStart records run start and returns previous skip/resume state.
func (t *SkipTracker) MarkRunStart(now time.Time) ResumeSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()

	snapshot := ResumeSnapshot{
		Skipped:    t.skipped,
		RunStarted: t.runStarted,
		RunStopped: t.runStopped,
	}
	t.skipped = 0
	t.runStarted = now
	return snapshot
}

// MarkRunStop records run completion time.
func (t *SkipTracker) MarkRunStop(now time.Time) {
	t.mu.Lock()
	t.runStopped = now
	t.mu.Unlock()
}
