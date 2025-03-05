// SPDX-License-Identifier: GPL-3.0-or-later

package k8ssd

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/cache"
)

const (
	startWaitTimeout  = time.Second * 3
	finishWaitTimeout = time.Second * 5
)

type discoverySim struct {
	td               *KubeDiscoverer
	runAfterSync     func(ctx context.Context)
	sortBeforeVerify bool
	wantTargetGroups []model.TargetGroup
}

func (sim discoverySim) run(t *testing.T) []model.TargetGroup {
	t.Helper()
	require.NotNil(t, sim.td)
	require.NotEmpty(t, sim.wantTargetGroups)

	in, out := make(chan []model.TargetGroup), make(chan []model.TargetGroup)
	go sim.collectTargetGroups(t, in, out)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	go sim.td.Discover(ctx, in)

	select {
	case <-sim.td.started:
	case <-time.After(startWaitTimeout):
		t.Fatalf("td %s failed to start in %s", sim.td.discoverers, startWaitTimeout)
	}

	synced := cache.WaitForCacheSync(ctx.Done(), sim.td.hasSynced)
	require.Truef(t, synced, "td %s failed to sync", sim.td.discoverers)

	if sim.runAfterSync != nil {
		sim.runAfterSync(ctx)
	}

	groups := <-out

	if sim.sortBeforeVerify {
		sortTargetGroups(groups)
	}

	sim.verifyResult(t, groups)
	return groups
}

func (sim discoverySim) collectTargetGroups(t *testing.T, in, out chan []model.TargetGroup) {
	var tggs []model.TargetGroup
loop:
	for {
		select {
		case inGroups := <-in:
			if tggs = append(tggs, inGroups...); len(tggs) >= len(sim.wantTargetGroups) {
				break loop
			}
		case <-time.After(finishWaitTimeout):
			t.Logf("td %s timed out after %s, got %d groups, expected %d, some events are skipped",
				sim.td.discoverers, finishWaitTimeout, len(tggs), len(sim.wantTargetGroups))
			break loop
		}
	}
	out <- tggs
}

func (sim discoverySim) verifyResult(t *testing.T, result []model.TargetGroup) {
	var expected, actual any

	if len(sim.wantTargetGroups) == len(result) {
		expected = sim.wantTargetGroups
		actual = result
	} else {
		want := make(map[string]model.TargetGroup)
		for _, group := range sim.wantTargetGroups {
			want[group.Source()] = group
		}
		got := make(map[string]model.TargetGroup)
		for _, group := range result {
			got[group.Source()] = group
		}
		expected, actual = want, got
	}

	assert.Equal(t, expected, actual)
}

type hasSynced interface {
	hasSynced() bool
}

var (
	_ hasSynced = &KubeDiscoverer{}
	_ hasSynced = &podDiscoverer{}
	_ hasSynced = &serviceDiscoverer{}
)

func (d *KubeDiscoverer) hasSynced() bool {
	for _, disc := range d.discoverers {
		v, ok := disc.(hasSynced)
		if !ok || !v.hasSynced() {
			return false
		}
	}
	return true
}

func (p *podDiscoverer) hasSynced() bool {
	return p.podInformer.HasSynced() && p.cmapInformer.HasSynced() && p.secretInformer.HasSynced()
}

func (s *serviceDiscoverer) hasSynced() bool {
	return s.informer.HasSynced()
}

func sortTargetGroups(tggs []model.TargetGroup) {
	if len(tggs) == 0 {
		return
	}
	sort.Slice(tggs, func(i, j int) bool { return tggs[i].Source() < tggs[j].Source() })
}
