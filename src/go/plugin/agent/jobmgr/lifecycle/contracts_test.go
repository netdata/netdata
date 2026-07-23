// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

var _ interface {
	Validate() error
	Class() LongLivedClass
} = LongLivedPlan{}

func TestControlFramePlanValidate(t *testing.T) {
	tests := map[string]struct {
		plan    ControlFramePlan
		wantErr bool
	}{
		"valid": {plan: ControlFramePlan{
			UID:    "request-1",
			Status: ControlDeadline,
			Expiry: 1,
		}},
		"empty UID": {plan: ControlFramePlan{
			Status: ControlDeadline,
			Expiry: 1,
		}, wantErr: true},
		"unsafe UID": {plan: ControlFramePlan{
			UID:    "request 1",
			Status: ControlDeadline,
			Expiry: 1,
		}, wantErr: true},
		"overlong UID": {
			plan: ControlFramePlan{
				UID:    strings.Repeat("u", MaximumUIDBytes+1),
				Status: ControlDeadline,
				Expiry: 1,
			},
			wantErr: true,
		},
		"unknown status": {plan: ControlFramePlan{
			UID:    "request-1",
			Status: 200,
			Expiry: 1,
		}, wantErr: true},
		"nonpositive expiry": {plan: ControlFramePlan{
			UID:    "request-1",
			Status: ControlDeadline,
		}, wantErr: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.wantErr, tc.plan.Validate() != nil)
		})
	}
}

func TestValidateUID(t *testing.T) {
	tests := map[string]struct {
		uid     string
		wantErr bool
	}{
		"valid":           {uid: "request-1"},
		"boundary":        {uid: strings.Repeat("u", MaximumUIDBytes)},
		"empty":           {wantErr: true},
		"overlong":        {uid: strings.Repeat("u", MaximumUIDBytes+1), wantErr: true},
		"space":           {uid: "request 1", wantErr: true},
		"tab":             {uid: "request\t1", wantErr: true},
		"carriage return": {uid: "request\r1", wantErr: true},
		"line feed":       {uid: "request\n1", wantErr: true},
		"NUL":             {uid: "request\x001", wantErr: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.wantErr, ValidateUID(tc.uid) != nil)
		})
	}
}

func TestClosedValues(t *testing.T) {
	assert.True(t, SourceJobManager.Valid())
	assert.True(t, SourceFunction.Valid())
	assert.False(t, Source(0).Valid())

	assert.True(t, TaskClassFrameworkControl.Valid())
	assert.True(t, TaskClassGenericFunction.Valid())
	assert.False(t, TaskClass(0).Valid())

	assert.True(t, (ResourceIdentity{
		ID:         "job:a",
		Generation: 1,
	}).Valid())
	assert.False(t, (ResourceIdentity{
		ID: "job:a",
	}).Valid())
	assert.False(t, (ResourceIdentity{
		Generation: 1,
	}).Valid())
}
