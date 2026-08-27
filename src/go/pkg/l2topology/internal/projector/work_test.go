// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"errors"
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
	"github.com/stretchr/testify/require"
)

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
