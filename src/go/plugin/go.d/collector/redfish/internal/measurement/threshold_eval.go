// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import (
	"slices"
	"time"
)

type thresholdState struct {
	known       bool
	active      bool
	breachSince time.Time
	clearSince  time.Time
}

type readingThresholdState struct {
	definitions []threshold
	states      []thresholdState
	epoch       string
	observedAt  time.Time
	cycle       uint64
}

// ResetDerivedHealth discards temporal evidence after an unobserved or discarded cycle.
// Like Project, it is called only by the serialized collector lifecycle.
func (c *Projector) ResetDerivedHealth() {
	clear(c.thresholdStates)
}

func (c *Projector) pruneThresholdStates() {
	for key, state := range c.thresholdStates {
		if state.cycle != c.thresholdCycle {
			delete(c.thresholdStates, key)
		}
	}
}

func (c *Projector) deriveHealth(reading normalizedReading, source *thresholdSource, now time.Time) string {
	if source == nil || reading.Metric == "" {
		return ""
	}
	definitions, valid := source.thresholds(reading.Family)
	if !valid {
		delete(c.thresholdStates, reading.Key)
		return "unavailable"
	}
	if len(definitions) == 0 {
		delete(c.thresholdStates, reading.Key)
		return ""
	}
	if !reading.Valid || !source.enabled() {
		delete(c.thresholdStates, reading.Key)
		return "unavailable"
	}

	// Instantaneous comparisons need no retained state. Only advertised temporal
	// behavior or a clearing offset justifies remembering previous observations.
	temporal := false
	for _, def := range definitions {
		temporal = temporal || def.dwellSeconds != 0 || def.clearSeconds != 0 || def.clearOffset != 0
	}
	var history *readingThresholdState
	if temporal {
		epoch := readingRateEpoch(reading)
		history = c.thresholdStates[reading.Key]
		if history == nil || history.epoch != epoch || !slices.Equal(history.definitions, definitions) ||
			!now.After(history.observedAt) {
			history = &readingThresholdState{
				definitions: definitions,
				states:      make([]thresholdState, len(definitions)),
				epoch:       epoch,
			}
			if c.thresholdStates == nil {
				c.thresholdStates = make(map[string]*readingThresholdState)
			}
			c.thresholdStates[reading.Key] = history
		}
		history.observedAt, history.cycle = now, c.thresholdCycle
	} else {
		delete(c.thresholdStates, reading.Key)
	}

	severity, unknownSeverity := 0, 0
	for i, def := range definitions {
		var instantaneous thresholdState
		state := &instantaneous
		if history != nil {
			state = &history.states[i]
		}
		state.observe(def, reading.Value, now)
		if !state.known {
			unknownSeverity = max(unknownSeverity, def.severity)
		} else if state.active {
			severity = max(severity, def.severity)
		}
	}
	// A known Critical result cannot be downgraded by an unknown lower threshold.
	if unknownSeverity > severity {
		return "unavailable"
	}
	return [...]string{"ok", "warning", "critical"}[severity]
}

func (s *thresholdState) observe(t threshold, value float64, now time.Time) {
	violates := value < t.limit
	clearedOffset := value >= t.limit+t.clearOffset
	if t.upper {
		violates = value > t.limit
		clearedOffset = value <= t.limit+t.clearOffset
	}
	if violates {
		s.clearSince = time.Time{}
		if s.breachSince.IsZero() {
			s.breachSince = now
		}
		if now.Sub(s.breachSince).Seconds() >= t.dwellSeconds {
			s.known, s.active = true, true
		}
		return
	}
	s.breachSince = time.Time{}
	if s.clearSince.IsZero() {
		s.clearSince = now
	}
	// The clear timer starts at the original limit, independently of the offset.
	if clearedOffset && now.Sub(s.clearSince).Seconds() >= t.clearSeconds {
		s.known, s.active = true, false
	}
}
