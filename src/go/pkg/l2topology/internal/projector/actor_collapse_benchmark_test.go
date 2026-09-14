// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
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
			template := projectedActorsWithSharedPrimary(tc.actors, tc.aliases)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				actors := cloneProjectedActorsForCollapse(template)
				actors = collapseActorsByIP(actors)
				runtime.KeepAlive(actors)
			}
		})
	}
}

func projectedActorsWithSharedPrimary(actorCount, aliasCount int) []projectedActor {
	actors := make([]projectedActor, 0, actorCount)
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
		actors = append(actors, projectedActor{
			Actor: graph.Actor{
				ActorID:   fmt.Sprintf("device-%d", actorIndex),
				ActorType: "device",
				Match: graph.Match{
					ChassisIDs:  []string{fmt.Sprintf("chassis-%d", actorIndex)},
					IPAddresses: addresses,
				},
			},
			Detail: model.ProjectionActorDetail{Device: model.ProjectionDeviceActorDetail{
				DeviceID:            fmt.Sprintf("device-%d", actorIndex),
				ManagementIP:        "172.16.0.1",
				ManagementAddresses: append([]string(nil), addresses...),
			}},
		})
	}
	return actors
}

func cloneProjectedActorsForCollapse(src []projectedActor) []projectedActor {
	out := append([]projectedActor(nil), src...)
	for i := range out {
		out[i].Actor.Match.ChassisIDs = append([]string(nil), src[i].Actor.Match.ChassisIDs...)
		out[i].Actor.Match.IPAddresses = append([]string(nil), src[i].Actor.Match.IPAddresses...)
		out[i].Detail.Device.ManagementAddresses = append([]string(nil), src[i].Detail.Device.ManagementAddresses...)
	}
	return out
}
