// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"errors"
	"fmt"
	"net/netip"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
	"github.com/stretchr/testify/require"
)

func TestToGraphStopsWorkImmediatelyAfterLimiterRejection(t *testing.T) {
	result := model.Result{
		CollectedAt: time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC),
		Devices: []model.Device{
			{ID: "switch-a", ManagementIP: netip.MustParseAddr("192.0.2.1")},
			{ID: "switch-b", ManagementIP: netip.MustParseAddr("192.0.2.2")},
		},
	}
	limitErr := errors.New("topology work exhausted")

	for rejectAt := 1; rejectAt < 1_000; rejectAt++ {
		charges := 0
		rejected := false
		callsAfterRejection := 0
		_, err := ToGraph(result, model.GraphOptions{
			CollectedAt: result.CollectedAt,
			ResolveDNSName: func(string) string {
				if rejected {
					callsAfterRejection++
				}
				return ""
			},
			WorkLimiter: func(uint64) error {
				charges++
				if charges == rejectAt {
					rejected = true
					return limitErr
				}
				return nil
			},
		})
		if err == nil {
			return
		}
		require.ErrorIs(t, err, limitErr)
		require.Zero(t, callsAfterRejection, "rejecting charge %d allowed later DNS work", rejectAt)
	}
	t.Fatal("did not reach an unrestricted graph build")
}

func TestInferFDBPairwiseBridgeLinksRejectsDenseFanoutBeforeMaterialization(t *testing.T) {
	const size = 8
	attachments := make([]model.Attachment, size)
	reporterAliases := make(map[string][]string, size)
	for i := range size {
		reporterAliases[fmt.Sprintf("switch-%d", i)] = []string{fmt.Sprintf("02:00:00:00:00:%02x", i)}
	}

	limitErr := errors.New("pairwise work exhausted")
	var charged []uint64
	work := &projectionWork{limiter: func(units uint64) error {
		charged = append(charged, units)
		return limitErr
	}}
	links := inferFDBPairwiseBridgeLinksWithWork(work, attachments, nil, reporterAliases)

	require.Nil(t, links)
	require.ErrorIs(t, work.err, limitErr)
	require.Equal(t, []uint64{size * size}, charged)
}
