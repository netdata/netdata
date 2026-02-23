// SPDX-License-Identifier: GPL-3.0-or-later

package tickstate

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSkipTrackerScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"skip and resume snapshots are consistent": {
			run: func(t *testing.T) {
				var tr SkipTracker
				start := time.Unix(10, 0)
				stop := time.Unix(12, 0)
				next := time.Unix(20, 0)

				s1 := tr.MarkSkipped()
				assert.Equal(t, 1, s1.Count)
				assert.True(t, s1.RunStarted.IsZero())

				tr.MarkRunStart(start)
				tr.MarkRunStop(stop)

				s2 := tr.MarkSkipped()
				assert.Equal(t, 1, s2.Count)
				assert.Equal(t, start, s2.RunStarted)

				s3 := tr.MarkSkipped()
				assert.Equal(t, 2, s3.Count)
				assert.Equal(t, start, s3.RunStarted)

				resume := tr.MarkRunStart(next)
				assert.Equal(t, 2, resume.Skipped)
				assert.Equal(t, start, resume.RunStarted)
				assert.Equal(t, stop, resume.RunStopped)

				s4 := tr.MarkSkipped()
				assert.Equal(t, 1, s4.Count)
				assert.Equal(t, next, s4.RunStarted)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}
