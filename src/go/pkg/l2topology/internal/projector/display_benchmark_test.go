// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
)

func BenchmarkApplyTopologyDisplayNamesAliasRichDevices(b *testing.B) {
	const (
		deviceCount = 64
		aliasCount  = 256
	)

	handles := graph.NewActorHandleAllocator()
	actors := make([]projectedActor, 0, deviceCount)
	for deviceIndex := range deviceCount {
		addresses := make([]string, 0, aliasCount)
		for aliasIndex := range aliasCount {
			addresses = append(addresses, fmt.Sprintf("10.%d.%d.%d",
				deviceIndex+1,
				aliasIndex/254,
				aliasIndex%254+1,
			))
		}
		actors = append(actors, projectedActor{
			Actor: graph.Actor{
				ActorHandle: handles.Next(),
				ActorType:   "device",
				Match: graph.Match{
					IPAddresses: addresses,
					SysName:     fmt.Sprintf("device-%d", deviceIndex+1),
				},
			},
			Detail: model.ProjectionActorDetail{
				Device: model.ProjectionDeviceActorDetail{ManagementIP: addresses[0]},
			},
		})
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		working := append([]projectedActor(nil), actors...)
		applyTopologyDisplayNames(working, nil, func(string) string { return "" })
	}
}
