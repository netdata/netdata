// SPDX-License-Identifier: GPL-3.0-or-later

package hostsocket

import (
	"context"
	"errors"
	"testing"

	"github.com/netdata/netdata/go/go.d.plugin/agent/discovery/sd/model"
)

var (
	localListenersOutputSample = []byte(`
UDP6|::1|8125|/opt/netdata/usr/sbin/netdata -P /run/netdata/netdata.pid -D
TCP6|::1|8125|/opt/netdata/usr/sbin/netdata -P /run/netdata/netdata.pid -D
TCP|127.0.0.1|8125|/opt/netdata/usr/sbin/netdata -P /run/netdata/netdata.pid -D
UDP|127.0.0.1|53768|/opt/netdata/usr/libexec/netdata/plugins.d/go.d.plugin 1
`)
)

func TestNetSocketDiscoverer_Discover(t *testing.T) {
	tests := map[string]discoverySim{
		"valid response": {
			mock:                 &mockLocalListenersExec{},
			wantDoneBeforeCancel: false,
			wantTargetGroups: []model.TargetGroup{&netSocketTargetGroup{
				provider: "hostsocket",
				source:   "net",
				targets: []model.Target{
					withHash(&NetSocketTarget{
						Protocol: "UDP6",
						Address:  "::1",
						Port:     "8125",
						Comm:     "netdata",
						Cmdline:  "/opt/netdata/usr/sbin/netdata -P /run/netdata/netdata.pid -D",
					}),
					withHash(&NetSocketTarget{
						Protocol: "TCP6",
						Address:  "::1",
						Port:     "8125",
						Comm:     "netdata",
						Cmdline:  "/opt/netdata/usr/sbin/netdata -P /run/netdata/netdata.pid -D",
					}),
					withHash(&NetSocketTarget{
						Protocol: "TCP",
						Address:  "127.0.0.1",
						Port:     "8125",
						Comm:     "netdata",
						Cmdline:  "/opt/netdata/usr/sbin/netdata -P /run/netdata/netdata.pid -D",
					}),
					withHash(&NetSocketTarget{
						Protocol: "UDP",
						Address:  "127.0.0.1",
						Port:     "53768",
						Comm:     "go.d.plugin",
						Cmdline:  "/opt/netdata/usr/libexec/netdata/plugins.d/go.d.plugin 1",
					}),
				},
			}},
		},
		"empty response": {
			mock:                 &mockLocalListenersExec{emptyResponse: true},
			wantDoneBeforeCancel: false,
			wantTargetGroups: []model.TargetGroup{&netSocketTargetGroup{
				provider: "hostsocket",
				source:   "net",
			}},
		},
		"error on exec": {
			mock:                 &mockLocalListenersExec{err: true},
			wantDoneBeforeCancel: true,
			wantTargetGroups:     nil,
		},
		"invalid data": {
			mock:                 &mockLocalListenersExec{invalidResponse: true},
			wantDoneBeforeCancel: true,
			wantTargetGroups:     nil,
		},
	}

	for name, sim := range tests {
		t.Run(name, func(t *testing.T) {
			sim.run(t)
		})
	}
}

func withHash(l *NetSocketTarget) *NetSocketTarget {
	l.hash, _ = calcHash(l)
	tags, _ := model.ParseTags("hostsocket net")
	l.Tags().Merge(tags)
	return l
}

type mockLocalListenersExec struct {
	err             bool
	emptyResponse   bool
	invalidResponse bool
}

func (m *mockLocalListenersExec) discover(context.Context) ([]byte, error) {
	if m.err {
		return nil, errors.New("mock discover() error")
	}
	if m.emptyResponse {
		return nil, nil
	}
	if m.invalidResponse {
		return []byte("this is very incorrect data"), nil
	}
	return localListenersOutputSample, nil
}
