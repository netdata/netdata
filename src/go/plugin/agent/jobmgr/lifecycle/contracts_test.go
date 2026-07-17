// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestControlFramePlanValidate(t *testing.T) {
	tests := map[string]struct {
		plan    ControlFramePlan
		wantErr bool
	}{
		"valid": {
			plan: ControlFramePlan{UID: "request-1", Status: ControlDeadline, Expiry: 1},
		},
		"empty UID": {
			plan:    ControlFramePlan{Status: ControlDeadline, Expiry: 1},
			wantErr: true,
		},
		"unsafe UID": {
			plan:    ControlFramePlan{UID: "request 1", Status: ControlDeadline, Expiry: 1},
			wantErr: true,
		},
		"unknown status": {
			plan:    ControlFramePlan{UID: "request-1", Status: 200, Expiry: 1},
			wantErr: true,
		},
		"nonpositive expiry": {
			plan:    ControlFramePlan{UID: "request-1", Status: ControlDeadline},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.wantErr, tc.plan.Validate() != nil)
		})
	}
}

func TestClosedValues(t *testing.T) {
	assert.True(t, SourceJobManager.Valid())
	assert.True(t, SourceFunction.Valid())
	assert.False(t, Source(0).Valid())

	assert.True(t, CapabilityApplied.Valid())
	assert.True(t, CapabilityDisposed.Valid())
	assert.True(t, CapabilityRetained.Valid())
	assert.False(t, CapabilityDisposition(0).Valid())

	assert.True(t, (ResourceIdentity{ID: "job:a", Generation: 1}).Valid())
	assert.False(t, (ResourceIdentity{ID: "job:a"}).Valid())
	assert.False(t, (ResourceIdentity{Generation: 1}).Valid())
}
