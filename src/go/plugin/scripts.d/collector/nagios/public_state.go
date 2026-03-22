// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

const (
	metricJobStateOK       = "ok"
	metricJobStateWarning  = "warning"
	metricJobStateCritical = "critical"
	metricJobStateUnknown  = "unknown"
	metricJobStateTimeout  = "timeout"
	metricJobStatePaused   = "paused"
	metricStateRetry       = "retry"
)

var jobExecutionStateNames = []string{
	metricJobStateOK,
	metricJobStateWarning,
	metricJobStateCritical,
	metricJobStateUnknown,
	metricJobStateTimeout,
	metricJobStatePaused,
	metricStateRetry,
}

// projectJobExecutionState maps runtime state into the public metric surface.
func projectJobExecutionState(state string, retrying bool) metrix.StateSetPoint {
	normalized := normalizeJobStateForMetric(state)
	actives := []string{normalized}
	if retrying && normalized != metricJobStatePaused {
		actives = append(actives, metricStateRetry)
	}
	return stateSetPoint(jobExecutionStateNames, actives...)
}

// projectPerfThresholdAlertState maps routed threshold state into the alertable duplicate.
func projectPerfThresholdAlertState(state string, retrying bool) metrix.StateSetPoint {
	if state == "" {
		return perfThresholdAlertStatePoint()
	}
	actives := []string{state}
	if retrying {
		actives = append(actives, perfThresholdStateRetry)
	}
	return perfThresholdAlertStatePoint(actives...)
}

func normalizeJobStateForMetric(state string) string {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case nagiosStateOK:
		return metricJobStateOK
	case nagiosStateWarning:
		return metricJobStateWarning
	case nagiosStateCritical:
		return metricJobStateCritical
	case jobStateTimeout:
		return metricJobStateTimeout
	case jobStatePaused:
		return metricJobStatePaused
	default:
		return metricJobStateUnknown
	}
}
