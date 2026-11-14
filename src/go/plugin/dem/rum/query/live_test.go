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
						Site:         observation.site,
						ExperienceID: observation.page,
						Events:       []beacon.Event{{Kind: beacon.EventDocument, ID: observation.page, Revision: 1}},
						SessionID:    observation.page,
						PageGroup:    observation.page,
						Received:     observation.received,
					},
				)
			}
			service := query.New(sites, nil)
			seen := map[string]bool{}
			sequences := map[string]uint64{}
			var cursor string
			for range 4 {
				rows, next, err := service.Live(context.Background(), "", cursor, limit, "")
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

func TestLiveVnodeSelectionUsesRecordedRowsBeforeLimits(t *testing.T) {
	const guidA = "11111111-1111-4111-8111-111111111111"
	const guidB = "22222222-2222-4222-8222-222222222222"
	hub := registry.New()
	a, _ := addSite(t, hub, "job", "generation")
	service := query.New(hub, nil)
	a.SetVnodeGUID(guidA)
	page(a, "job", "/a-old")
	a.SetVnodeGUID(guidB)
	page(a, "job", "/b-one")
	page(a, "job", "/b-two")
	a.SetVnodeGUID(guidA)
	page(a, "job", "/a-new")
	ctx := context.Background()
	selected, cursor, err := service.Live(ctx, "job", "", 1, guidB)
	require.NoError(t, err)
	require.Len(t, selected, 1)
	assert.Equal(t, "/b-one", selected[0].Page)
	selected, cursor, err = service.Live(ctx, "job", cursor, 1, guidB)
	require.NoError(t, err)
	require.Len(t, selected, 1)
	assert.Equal(t, "/b-two", selected[0].Page)
	selected, cursor, err = service.Live(ctx, "job", cursor, 1, guidA)
	require.NoError(t, err)
	require.Len(t, selected, 1)
	assert.Equal(t, "/a-old", selected[0].Page, "changing GUID selection starts the retained view")
	selected, _, err = service.Live(ctx, "job", cursor, 1, guidA)
	require.NoError(t, err)
	require.Len(t, selected, 1)
	assert.Equal(t, "/a-new", selected[0].Page)
	sites, err := service.Sites(ctx, guidB)
	require.NoError(t, err)
	assert.Empty(t, sites, "inventory reports current identity rather than old buffered identity")
	pages, err := service.Pages(ctx, "job", guidB)
	require.NoError(t, err)
	assert.Empty(t, pages, "rolling pages belong only to the current measurement window")
}

func TestLivePreservesQuietSitesAndPaginatesWithoutSkipping(t *testing.T) {
	hub := registry.New()
	s := query.New(hub, nil)
	hot, _ := addSite(t, hub, "hot", "first")
	quiet, _ := addSite(t, hub, "quiet", "first")
	for i := 0; i < 6; i++ {
		page(hot, "hot", fmt.Sprintf("/hot/%d", i))
	}
	page(quiet, "quiet", "/quiet/first")
	var paths []string
	var cursor string
	for range 4 {
		rows, next, err := s.Live(context.Background(), "", cursor, 2, "")
		require.NoError(t, err)
		cursor = next
		for _, r := range rows {
			paths = append(paths, r.Page)
		}
	}
	assert.Equal(t, []string{"/hot/0", "/hot/1", "/hot/2", "/hot/3", "/hot/4", "/hot/5", "/quiet/first"}, paths)
	page(quiet, "quiet", "/quiet/second")
	rows, next, err := s.Live(context.Background(), "", cursor, 2, "")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "/quiet/second", rows[0].Page)
	rows, _, err = s.Live(context.Background(), "", next, 2, "")
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestLiveReplacementAndFilterDoNotReuseAnotherStreamPosition(t *testing.T) {
	hub := registry.New()
	s := query.New(hub, nil)
	a, retire := addSite(t, hub, "a", "old")
	b, _ := addSite(t, hub, "b", "other")
	page(a, "a", "/old")
	page(b, "b", "/other")
	rows, cursor, err := s.Live(context.Background(), "a", "", 10, "")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	retire()
	replacement, _ := addSite(t, hub, "a", "new")
	page(replacement, "a", "/new")
	rows, _, err = s.Live(context.Background(), "", cursor, 10, "")
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "/other", rows[0].Page)
	assert.Equal(t, "/new", rows[1].Page)
	assert.Equal(t, "new", rows[1].Generation)
}
