// SPDX-License-Identifier: GPL-3.0-or-later

package agenthost

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent"
	"github.com/stretchr/testify/assert"
)

func TestRestartControlErrorTreatsStoppedProcessAsBenign(t *testing.T) {
	sentinel := errors.New("restart failed")
	tests := map[string]struct {
		err  error
		want error
	}{
		"success": {},
		"process already stopped": {
			err: agent.ErrNotRunning,
		},
		"wrapped process already stopped": {
			err: errors.Join(sentinel, agent.ErrNotRunning),
		},
		"restart failure": {
			err:  sentinel,
			want: sentinel,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.ErrorIs(t, restartControlError(test.err), test.want)
		})
	}
}

func TestWaitForRunReturnsExactTerminalDisposition(t *testing.T) {
	sentinel := errors.New("dirty run")
	tests := map[string]struct {
		done    chan error
		timeout time.Duration
		want    error
		match   string
	}{
		"clean": {
			done:    completedRun(nil),
			timeout: time.Second,
		},
		"dirty": {
			done:    completedRun(sentinel),
			timeout: time.Second,
			want:    sentinel,
		},
		"timeout": {
			done:    make(chan error),
			timeout: time.Millisecond,
			match:   "timed out",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := waitForRun(test.done, test.timeout)
			if test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("terminal error=%v, want %v", err, test.want)
			}
			if test.match != "" && (err == nil || !strings.Contains(err.Error(), test.match)) {
				t.Fatalf("terminal error=%v, want match %q", err, test.match)
			}
			if test.want == nil && test.match == "" && err != nil {
				t.Fatalf("clean terminal returned %v", err)
			}
		})
	}
}

func completedRun(err error) chan error {
	done := make(chan error, 1)
	done <- err
	return done
}
