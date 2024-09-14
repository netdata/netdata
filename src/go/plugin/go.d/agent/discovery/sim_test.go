// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type discoverySim struct {
	mgr            *Manager
	collectDelay   time.Duration
	expectedGroups []*confgroup.Group
}

func (sim discoverySim) run(t *testing.T) {
	t.Helper()
	require.NotNil(t, sim.mgr)

	in, out := make(chan []*confgroup.Group), make(chan []*confgroup.Group)
	go sim.collectGroups(t, in, out)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sim.mgr.Run(ctx, in)

	actualGroups := <-out

	sortGroups(sim.expectedGroups)
	sortGroups(actualGroups)

	assert.Equal(t, sim.expectedGroups, actualGroups)
}

func (sim discoverySim) collectGroups(t *testing.T, in, out chan []*confgroup.Group) {
	time.Sleep(sim.collectDelay)

	timeout := sim.mgr.sendEvery + time.Second*2
	var groups []*confgroup.Group
loop:
	for {
		select {
		case inGroups := <-in:
			if groups = append(groups, inGroups...); len(groups) >= len(sim.expectedGroups) {
				break loop
			}
		case <-time.After(timeout):
			t.Logf("discovery %s timed out after %s, got %d groups, expected %d, some events are skipped",
				sim.mgr.discoverers, timeout, len(groups), len(sim.expectedGroups))
			break loop
		}
	}
	out <- groups
}

func sortGroups(groups []*confgroup.Group) {
	if len(groups) == 0 {
		return
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Source < groups[j].Source })
}
