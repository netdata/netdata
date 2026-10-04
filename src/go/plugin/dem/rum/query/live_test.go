// SPDX-License-Identifier: GPL-3.0-or-later
package query_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLiveOutOfOrderReceivedTimesDoNotSkipOrReplay(t *testing.T) {
	for _, limit := range []int{2, 3} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			sites := registry.New()
			a, _ := addSite(t, sites, "a", "first")
			b, _ := addSite(t, sites, "b", "first")
			now := time.Now()
			// Concurrent HTTP normalization can finish in a different order from receipt.
			for _, observation := range []struct {
				site, page string
				received   time.Time
			}{
				{"a", "/a1", now.Add(-time.Second)},
				{"a", "/a2", now.Add(-3 * time.Second)},
				{"b", "/b1", now.Add(-2 * time.Second)},
			} {
				owner := a
				if observation.site == "b" {
					owner = b
				}
				owner.Ingest(
					&beacon.Beacon{
						Site:      observation.site,
						SessionID: observation.page,
						PageGroup: observation.page,
						Received:  observation.received,
					},
				)
			}
			service := query.New(sites, nil)
			seen := map[string]bool{}
			sequences := map[string]uint64{}
			var cursor string
			for range 4 {
				rows, next, err := service.Live(context.Background(), "", cursor, limit)
				require.NoError(t, err)
				cursor = next
				for _, row := range rows {
					require.False(t, seen[row.Page], "replayed %s", row.Page)
					assert.Equal(
						t,
						sequences[row.Site]+1,
						row.Seq,
						"each site must advance through its sequence prefix",
					)
					sequences[row.Site] = row.Seq
					seen[row.Page] = true
				}
			}
			assert.Equal(t, map[string]bool{"/a1": true, "/a2": true, "/b1": true}, seen)
		})
	}
}
