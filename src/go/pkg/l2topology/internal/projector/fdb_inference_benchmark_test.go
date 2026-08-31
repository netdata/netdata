// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
)

func BenchmarkInferFDBPairwiseAmbiguousAliasFanout(b *testing.B) {
	for _, ownerCount := range []int{8, 64, 256} {
		rowCount := ownerCount * 16
		attachments := make([]model.Attachment, 0, rowCount)
		ifaceByDeviceIndex := make(map[string]model.Interface, rowCount)
		for rowIndex := range rowCount {
			ifIndex := rowIndex + 1
			attachments = append(attachments, model.Attachment{
				DeviceID:   "source",
				IfIndex:    ifIndex,
				EndpointID: "mac:bb:bb:bb:bb:bb:bb",
				Method:     "fdb",
			})
			ifaceByDeviceIndex[deviceIfIndexKey("source", ifIndex)] = model.Interface{
				DeviceID: "source",
				IfIndex:  ifIndex,
				IfName:   fmt.Sprintf("Gi0/%d", ifIndex),
			}
		}
		reporterAliases := map[string][]string{
			"source": {"mac:aa:aa:aa:aa:aa:aa"},
		}
		for ownerIndex := range ownerCount {
			reporterAliases[fmt.Sprintf("owner-%d", ownerIndex)] = []string{"mac:bb:bb:bb:bb:bb:bb"}
		}

		b.Run(fmt.Sprintf("owners=%d/rows=%d", ownerCount, rowCount), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				records := inferFDBPairwiseBridgeLinks(attachments, ifaceByDeviceIndex, reporterAliases)
				if len(records) != 0 {
					b.Fatalf("inferred %d links from ambiguous aliases", len(records))
				}
				runtime.KeepAlive(records)
			}
		})
	}
}
