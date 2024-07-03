// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

type discoverySim struct {
	config            string
	discoverers       []model.Discoverer
	wantClassifyCalls int
	wantComposeCalls  int
	wantConfGroups    []*confgroup.Group
}

func (sim discoverySim) run(t *testing.T) {
	t.Helper()

	var cfg Config
	err := yaml.Unmarshal([]byte(sim.config), &cfg)
	require.Nilf(t, err, "cfg unmarshal")

	clr, err := newTargetClassificator(cfg.Classify)
	require.Nil(t, err, "newTargetClassificator")

	cmr, err := newConfigComposer(cfg.Compose)
	require.Nil(t, err, "newConfigComposer")

	mockClr := &mockClassificator{clr: clr}
	mockCmr := &mockComposer{cmr: cmr}

	accum := newAccumulator()
	accum.sendEvery = time.Second * 2

	pl := &Pipeline{
		Logger:      logger.New(),
		discoverers: sim.discoverers,
		accum:       accum,
		clr:         mockClr,
		cmr:         mockCmr,
		configs:     make(map[string]map[uint64][]confgroup.Config),
	}

	pl.accum.Logger = pl.Logger
	clr.Logger = pl.Logger
	cmr.Logger = pl.Logger

	groups := sim.collectGroups(t, pl)

	sortConfigGroups(groups)
	sortConfigGroups(sim.wantConfGroups)

	assert.Equal(t, sim.wantConfGroups, groups)
	assert.Equalf(t, sim.wantClassifyCalls, mockClr.calls, "classify calls")
	assert.Equalf(t, sim.wantComposeCalls, mockCmr.calls, "compose calls")
}

func (sim discoverySim) collectGroups(t *testing.T, pl *Pipeline) []*confgroup.Group {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	in := make(chan []*confgroup.Group)
	done := make(chan struct{})

	go func() { defer close(done); pl.Run(ctx, in) }()

	timeout := time.Second * 10
	var groups []*confgroup.Group

	func() {
		for {
			select {
			case inGroups := <-in:
				groups = append(groups, inGroups...)
			case <-done:
				return
			case <-time.After(timeout):
				t.Logf("discovery timed out after %s, got %d groups, expected %d, some events are skipped",
					timeout, len(groups), len(sim.wantConfGroups))
				return
			}
		}
	}()

	return groups
}

type mockClassificator struct {
	calls int
	clr   *targetClassificator
}

func (m *mockClassificator) classify(tgt model.Target) model.Tags {
	m.calls++
	return m.clr.classify(tgt)
}

type mockComposer struct {
	calls int
	cmr   *configComposer
}

func (m *mockComposer) compose(tgt model.Target) []confgroup.Config {
	m.calls++
	return m.cmr.compose(tgt)
}

func sortConfigGroups(groups []*confgroup.Group) {
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Source < groups[j].Source
	})

	for _, g := range groups {
		sort.Slice(g.Configs, func(i, j int) bool {
			return g.Configs[i].Name() < g.Configs[j].Name()
		})
	}
}
