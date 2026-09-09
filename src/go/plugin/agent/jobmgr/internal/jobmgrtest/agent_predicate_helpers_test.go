// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgrtest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWaitForOutputFaultPrefersObservedInjection(t *testing.T) {
	ready := func() <-chan struct{} {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	blocked := func() <-chan struct{} {
		return make(chan struct{})
	}
	tests := map[string]struct {
		injected    <-chan struct{}
		fixtureDone <-chan struct{}
		contextDone <-chan struct{}
		want        outputFaultWaitResult
	}{
		"injection": {
			injected: ready(), fixtureDone: blocked(), contextDone: blocked(),
			want: outputFaultInjectionObserved,
		},
		"fixture termination": {
			injected: blocked(), fixtureDone: ready(), contextDone: blocked(),
			want: outputFaultFixtureTerminated,
		},
		"context completion": {
			injected: blocked(), fixtureDone: blocked(), contextDone: ready(),
			want: outputFaultContextDone,
		},
		"injection and fixture termination": {
			injected: ready(), fixtureDone: ready(), contextDone: blocked(),
			want: outputFaultInjectionObserved,
		},
		"injection and context completion": {
			injected: ready(), fixtureDone: blocked(), contextDone: ready(),
			want: outputFaultInjectionObserved,
		},
		"all completed": {
			injected: ready(), fixtureDone: ready(), contextDone: ready(),
			want: outputFaultInjectionObserved,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			for range 1_024 {
				got := waitForOutputFault(test.injected, test.fixtureDone, test.contextDone)
				require.Equal(t, test.want, got)
			}
		})
	}
}
