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
	tests := map[string]struct {
		call functionwire.Call
	}{
		"collector method": {
			call: functionwire.Call{
				UID: "method", Method: "module:method",
				Timeout: time.Second,
			},
		},
		"DynCfg method": {
			call: functionwire.Call{
				UID: "dyncfg", Method: "config",
				Args:    []string{"go.d:collector:module:job", "enable"},
				Timeout: time.Second,
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			port := &recordingIngressPort{done: make(chan struct{})}
			ingress, err := NewIngress(
				port,
				lifecycle.RealClock{},
				func() {},
			)
			require.NoError(t, err)

			require.NoError(t, ingress.HandleCall(context.Background(), test.call))

			require.EqualValues(t, 1, len(port.requests))
			request := port.requests[0]
			require.EqualValues(t, lifecycle.SourceFunction, request.Source)
			require.EqualValues(t, "", request.LaneKey)
			require.EqualValues(t, test.call.Method, request.Route)
		})
	}
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
