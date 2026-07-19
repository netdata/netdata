// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	functionwire "github.com/netdata/netdata/go/plugins/plugin/framework/functions"
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
			if err != nil {
				t.Fatal(err)
			}
			if err := ingress.HandleCall(
				context.Background(),
				test.call,
			); err != nil {
				t.Fatal(err)
			}
			if len(port.requests) != 1 {
				t.Fatalf("submissions=%d want=1", len(port.requests))
			}
			request := port.requests[0]
			if request.Source != lifecycle.SourceFunction {
				t.Fatalf("source=%v want Function", request.Source)
			}
			if request.LaneKey != "" {
				t.Fatalf(
					"ingress preselected catalog resource %q",
					request.LaneKey,
				)
			}
			if request.Route != test.call.Method {
				t.Fatalf(
					"route=%q want=%q",
					request.Route,
					test.call.Method,
				)
			}
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
