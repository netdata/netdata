// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

func BenchmarkCollapseActorsByIPAliasCollisions(b *testing.B) {
	for _, tc := range []struct {
		actors  int
		aliases int
	}{
		{actors: 16, aliases: 64},
		{actors: 32, aliases: 64},
		{actors: 64, aliases: 64},
		{actors: 64, aliases: 256},
	} {
		b.Run(fmt.Sprintf("actors=%d/aliases=%d", tc.actors, tc.aliases), func(b *testing.B) {
			template := topologyActorsWithSharedPrimary(tc.actors, tc.aliases)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				data := topologymodel.Data{Actors: cloneTopologyActorsForCollapse(template)}
				collapseActorsByIP(&data)
				runtime.KeepAlive(data)
			}
		})
	}
}

func topologyActorsWithSharedPrimary(actorCount, aliasCount int) []topologymodel.Actor {
	actors := make([]topologymodel.Actor, 0, actorCount)
	for actorIndex := range actorCount {
		addresses := make([]string, 0, aliasCount+1)
		addresses = append(addresses, "172.16.0.1")
		for aliasIndex := range aliasCount {
			addresses = append(addresses, fmt.Sprintf(
				"10.%d.%d.%d",
				actorIndex+1,
				aliasIndex/254,
				aliasIndex%254+1,
			))
		}
		actors = append(actors, topologymodel.Actor{
			ActorID:   fmt.Sprintf("device-%d", actorIndex),
			ActorType: "device",
			Match: topologymodel.Match{
				ChassisIDs:  []string{fmt.Sprintf("chassis-%d", actorIndex)},
				IPAddresses: addresses,
			},
		})
	}
	return actors
}

func cloneTopologyActorsForCollapse(src []topologymodel.Actor) []topologymodel.Actor {
	out := append([]topologymodel.Actor(nil), src...)
	for i := range out {
		out[i].Match.ChassisIDs = append([]string(nil), src[i].Match.ChassisIDs...)
		out[i].Match.IPAddresses = append([]string(nil), src[i].Match.IPAddresses...)
	}
	return out
}
