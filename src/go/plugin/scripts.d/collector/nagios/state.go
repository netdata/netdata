// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"strings"
	"time"
)

type collectState struct {
	nextAnniversary time.Time
	nextDue         time.Time

	serviceState string
	jobState     string
	softAttempts int
	retrying     bool
	maxAttempts  int

	lastPerfValues          []perfValueMeasureSet
	lastPerfThresholdStates []perfThresholdStateSet
}

func newCollectState(now time.Time, job JobConfig) collectState {
	return collectState{
		nextAnniversary: now,
		nextDue:         now,
		serviceState:    nagiosStateUnknown,
		jobState:        nagiosStateUnknown,
		maxAttempts:     max(job.MaxCheckAttempts, 1),
	}
}

func (s *collectState) due(now time.Time) bool {
	return s.nextDue.IsZero() || !s.nextDue.After(now)
}

func (s *collectState) currentServiceState() string {
	if s == nil || s.serviceState == "" {
		return nagiosStateUnknown
	}
	return s.serviceState
}

func (s *collectState) currentJobState() string {
	if s == nil || s.jobState == "" {
		return nagiosStateUnknown
	}
	return s.jobState
}

func (s *collectState) isRetrying() bool {
	return s != nil && s.retrying
}

func (s *collectState) currentAttempt() int {
	if s == nil {
		return 1
	}
	if strings.EqualFold(s.serviceState, nagiosStateOK) || s.serviceState == "" {
		return 1
	}
	attempt := s.softAttempts
	if attempt <= 0 {
		attempt = 1
	}
	if s.retrying {
		attempt++
	}
	if attempt > s.maxAttempts {
		return s.maxAttempts
	}
	return attempt
}

func (s *collectState) macroState() macroState {
	return macroState{
		ServiceState:       s.currentServiceState(),
		ServiceAttempt:     s.currentAttempt(),
		ServiceMaxAttempts: s.maxAttempts,
	}
}

func (s *collectState) recordResult(serviceState, jobState string) {
	serviceState = normalizeState(serviceState)
	jobState = normalizeJobState(jobState)
	if serviceState == nagiosStateOK {
		s.softAttempts = 0
		s.retrying = false
	} else {
		s.softAttempts++
		if s.softAttempts >= s.maxAttempts {
			s.retrying = false
		} else {
			s.retrying = true
		}
	}
	s.serviceState = serviceState
	s.jobState = jobState
}

func (s *collectState) recordPeriodBlocked() {
	s.jobState = jobStatePaused
	for i := range s.lastPerfThresholdStates {
		s.lastPerfThresholdStates[i].state = ""
	}
}

func (s *collectState) rememberPerf(result perfRouteResult) {
	s.lastPerfValues = append(s.lastPerfValues[:0], result.values...)
	s.lastPerfThresholdStates = append(s.lastPerfThresholdStates[:0], result.thresholdStates...)
}

func (s *collectState) perfValueSets() []perfValueMeasureSet {
	return s.lastPerfValues
}

func (s *collectState) perfThresholdStates() []perfThresholdStateSet {
	return s.lastPerfThresholdStates
}

func (s *collectState) completeRun(now time.Time, serviceState, jobState string, perf perfRouteResult, job JobConfig) {
	s.recordResult(serviceState, jobState)
	s.rememberPerf(perf)
	if s.retrying {
		s.scheduleRetry(now, job.RetryInterval.Duration())
		return
	}
	s.scheduleRegular(now, s.nextAnniversary, job.CheckInterval.Duration())
}

func normalizeState(state string) string {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case nagiosStateOK:
		return nagiosStateOK
	case nagiosStateWarning:
		return nagiosStateWarning
	case nagiosStateCritical:
		return nagiosStateCritical
	default:
		return nagiosStateUnknown
	}
}

func normalizeJobState(state string) string {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case jobStateTimeout:
		return jobStateTimeout
	case jobStatePaused:
		return jobStatePaused
	default:
		return normalizeState(state)
	}
}
