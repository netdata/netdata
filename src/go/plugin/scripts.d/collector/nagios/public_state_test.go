// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProjectJobExecutionState(t *testing.T) {
	t.Run("paused suppresses retry", func(t *testing.T) {
		point := projectJobExecutionState(jobStatePaused, true)
		assert.True(t, point.States[metricJobStatePaused])
		assert.False(t, point.States[metricStateRetry])
	})

	t.Run("warning keeps retry", func(t *testing.T) {
		point := projectJobExecutionState(nagiosStateWarning, true)
		assert.True(t, point.States[metricJobStateWarning])
		assert.True(t, point.States[metricStateRetry])
	})
}

func TestProjectPerfThresholdAlertState(t *testing.T) {
	t.Run("empty state clears everything", func(t *testing.T) {
		point := projectPerfThresholdAlertState("", true)
		for _, state := range perfThresholdAlertStateNames {
			assert.False(t, point.States[state])
		}
	})

	t.Run("warning keeps retry", func(t *testing.T) {
		point := projectPerfThresholdAlertState(perfThresholdStateWarning, true)
		assert.True(t, point.States[perfThresholdStateWarning])
		assert.True(t, point.States[perfThresholdStateRetry])
	})
}
