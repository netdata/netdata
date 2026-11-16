// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	"github.com/stretchr/testify/assert"
)

func TestManagedFocusBindingsUseManagedReconciledOccurrences(t *testing.T) {
	handles := graph.NewActorHandleAllocator()
	first, second := handles.Next(), handles.Next()
	actors := []topologymodel.Actor{
		{ActorHandle: first, ActorID: "a", ActorType: "switch", Source: "snmp",
			Match: topologymodel.Match{IPAddresses: []string{"192.0.2.1", "::ffff:192.0.2.2"}}},
		{ActorHandle: second, ActorID: "b", ActorType: "router", Source: "snmp",
			Match: topologymodel.Match{IPAddresses: []string{"192.0.2.1"}}},
		{ActorHandle: handles.Next(), ActorID: "endpoint", ActorType: "endpoint", Source: "snmp",
			Match: topologymodel.Match{IPAddresses: []string{"192.0.2.1"}}},
		{ActorHandle: handles.Next(), ActorID: "remote", ActorType: "switch", Source: "lldp",
			Match: topologymodel.Match{IPAddresses: []string{"192.0.2.1"}}},
		{ActorID: "zero", ActorType: "switch", Source: "snmp",
			Match: topologymodel.Match{IPAddresses: []string{"192.0.2.1"}}},
	}
	targets := []topologyoptions.ManagedFocusTarget{
		{Value: "ip:192.0.2.1"}, {Value: "ip:192.0.2.2"}, {Value: "ip:192.0.2.1"},
		{Value: "ip:192.0.2.99"}, {Value: "all_devices"}, {Value: ""},
	}
	want := []topologymodel.FocusRootBinding{
		{OptionID: "ip:192.0.2.1", ActorHandle: first},
		{OptionID: "ip:192.0.2.1", ActorHandle: second},
		{OptionID: "ip:192.0.2.2", ActorHandle: first},
	}
	assert.Equal(t, want, BuildManagedFocusBindings(actors, targets))
	assert.Equal(t, want, BuildManagedFocusBindings(actors, targets), "order must not depend on address-map iteration")
}
