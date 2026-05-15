// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestServiceStateFromExecution(t *testing.T) {
	tests := map[string]struct {
		exitCode int
		err      error
		want     string
	}{
		"success maps to ok": {
			exitCode: 0,
			want:     nagiosStateOK,
		},
		"warning exit maps to warning even when err is non nil": {
			exitCode: 1,
			err:      errors.New("plugin returned warning"),
			want:     nagiosStateWarning,
		},
		"critical exit maps to critical even when err is non nil": {
			exitCode: 2,
			err:      errors.New("plugin returned critical"),
			want:     nagiosStateCritical,
		},
		"unknown exit maps to unknown even when err is non nil": {
			exitCode: 3,
			err:      errors.New("plugin returned unknown"),
			want:     nagiosStateUnknown,
		},
		"timeout maps to unknown service state": {
			exitCode: -1,
			err:      errNagiosCheckTimeout,
			want:     nagiosStateUnknown,
		},
		"deadline exceeded maps to unknown service state": {
			exitCode: -1,
			err:      context.DeadlineExceeded,
			want:     nagiosStateUnknown,
		},
		"non nagios exit defaults to unknown": {
			exitCode: 7,
			err:      errors.New("bad exit"),
			want:     nagiosStateUnknown,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, serviceStateFromExecution(tc.exitCode, tc.err))
		})
	}
}
