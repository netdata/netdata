// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"fmt"
	"testing"
)

// Finalization is O(nodes + retained relationships + restored members). Setup is
// excluded: this measures the cycle's restoration and authoritative pruning work.
// Time is a local trend; allocations must remain linear in retained membership.
func BenchmarkGraphMembershipFinalization(b *testing.B) {
	for _, count := range []int{32, 1024} {
		for _, removed := range []bool{false, true} {
			b.Run(fmt.Sprintf("members_%d/removed_%t", count, removed), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					b.StopTimer()
					client := &Client{
						graphMembership: make(map[string]graphMembershipSnapshot),
					}
					graph := &resourceGraph{
						Complete: false,
					}
					rel := graphRelationship{
						Path:      "Sensors",
						ChildKind: "sensor",
					}
					for i := 0; i < count; i++ {
						parent := graphTestNode(
							"chassis",
							fmt.Sprintf("parent-%d", i),
							fmt.Sprintf("/redfish/v1/Chassis/%d", i),
							nil,
						)
						child := graphTestNode("sensor", fmt.Sprintf("sensor-%d", i), parent.URI+"/Sensors/1", nil)
						client.graphMembership[graphMembershipKey(parent.Key, rel)] = graphMembershipSnapshot{
							ParentKey:    parent.Key,
							Relationship: rel,
							Members:      snapshotGraphMembers([]*graphNode{child}),
						}
						if !removed || i%4 == 0 {
							if err := graph.add(parent); err != nil {
								b.Fatal(err)
							}
						}
					}
					b.StartTimer()
					if err := client.finalizeGraphMembership(graph, true); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
