// SPDX-License-Identifier: GPL-3.0-or-later

package contract

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestObjectiveResultFor(t *testing.T) {
	objective := 10 * time.Second
	tests := map[string]struct {
		lag    time.Duration
		status Status
	}{
		"below objective": {lag: objective - time.Nanosecond, status: StatusSuccess},
		"at objective":    {lag: objective, status: StatusSuccess},
		"above objective": {lag: objective + time.Nanosecond, status: StatusFailed},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := ObjectiveResultFor(tc.lag, objective)

			assert.True(t, result.Performed)
			assert.Equal(t, tc.status, result.Status)
			assert.Equal(t, tc.lag, result.Lag)
		})
	}
}
