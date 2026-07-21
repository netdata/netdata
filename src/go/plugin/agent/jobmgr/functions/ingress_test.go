// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	functionwire "github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/stretchr/testify/require"
)

func TestFunctionIngressLeavesResourceSelectionToCatalog(t *testing.T) {
	now := time.Unix(100, 0)
	tests := map[string]struct {
		call         functionwire.Call
		wantDeadline time.Time
	}{
		"collector method": {
			call: functionwire.Call{
				UID: "method", Method: "module:method",
				Timeout: time.Second,
			},
			wantDeadline: now.Add(time.Second),
		},
		"DynCfg method without timeout": {
			call: functionwire.Call{
				UID: "dyncfg", Method: "config",
				Args: []string{"go.d:collector:module:job", "enable"},
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			port := &recordingIngressPort{done: make(chan struct{})}
			ingress, err := NewIngress(
				port,
				ingressTestClock{now: now},
				func() {},
			)
			require.NoError(t, err)

			require.NoError(t, ingress.HandleCall(context.Background(), test.call))
			if len(test.call.Args) != 0 {
				test.call.Args[0] = "mutated"
			}

			require.Len(t, port.requests, 1)
			request := port.requests[0]
			require.Equal(t, lifecycle.SourceFunction, request.Source)
			require.Empty(t, request.LaneKey)
			require.Equal(t, test.call.UID, request.UID)
			require.Equal(t, test.call.Method, request.Route)
			if len(test.call.Args) == 0 {
				require.Empty(t, request.Args)
			} else {
				require.Equal(t, []string{"go.d:collector:module:job", "enable"}, request.Args)
			}
			require.Equal(t, test.call.Timeout, request.Timeout)
			require.Equal(t, test.wantDeadline, request.Deadline)
		})
	}
}

type ingressTestClock struct {
	now time.Time
}

func (clock ingressTestClock) Now() time.Time {
	return clock.now
}

func (ingressTestClock) Arm(string, time.Duration) (<-chan time.Time, func()) {
	panic("unexpected timer arm")
}

type recordingIngressPort struct {
	requests []jobmgr.Request
	done     chan struct{}
}

func (rip *recordingIngressPort) Submit(
	_ context.Context,
	request jobmgr.Request,
) error {
	rip.requests = append(rip.requests, request)
	return nil
}

func (*recordingIngressPort) Reject(
	context.Context,
	string,
	lifecycle.ControlStatus,
) error {
	return nil
}

func (*recordingIngressPort) Cancel(context.Context, string) error {
	return nil
}

func (*recordingIngressPort) NotifyControlReady() {}

func (rip *recordingIngressPort) Done() <-chan struct{} {
	return rip.done
}
