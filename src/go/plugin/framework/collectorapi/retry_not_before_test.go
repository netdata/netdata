// SPDX-License-Identifier: GPL-3.0-or-later

package collectorapi

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRetryNotBefore(t *testing.T) {
	early := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	late := early.Add(time.Minute)
	cause := errors.New("provider wait")
	tests := map[string]struct {
		err      error
		want     time.Time
		class    LifecycleErrorClass
		hasCause bool
	}{
		"nil":           {},
		"unannotated":   {err: cause, hasCause: true},
		"zero":          {err: WithRetryNotBefore(cause, time.Time{}), hasCause: true},
		"wrapped":       {err: fmt.Errorf("check: %w", WithRetryNotBefore(cause, early)), want: early, hasCause: true},
		"latest nested": {err: WithRetryNotBefore(WithRetryNotBefore(cause, late), early), want: late, hasCause: true},
		"latest joined": {
			err:      errors.Join(WithRetryNotBefore(cause, early), WithRetryNotBefore(errors.New("other"), late)),
			want:     late,
			hasCause: true,
		},
		"temporary": {
			err:      TemporaryError(WithRetryNotBefore(cause, late)),
			want:     late,
			class:    LifecycleErrorTemporary,
			hasCause: true,
		},
		"permanent wins": {
			err:      WithRetryNotBefore(errors.Join(TemporaryError(cause), PermanentError(cause)), late),
			want:     late,
			class:    LifecycleErrorPermanent,
			hasCause: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, RetryNotBefore(tc.err))
			assert.Equal(t, tc.want, RetryNotBefore(tc.err), "repeated inspection preserves the captured deadline")
			assert.Equal(t, tc.class, ClassifyLifecycleError(tc.err))
			assert.Equal(t, tc.hasCause, errors.Is(tc.err, cause))
		})
	}
	assert.Nil(t, WithRetryNotBefore(nil, late))
	assert.Equal(t, cause.Error(), WithRetryNotBefore(cause, late).Error())
}
