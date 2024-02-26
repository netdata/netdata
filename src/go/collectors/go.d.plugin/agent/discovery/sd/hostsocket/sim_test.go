// SPDX-License-Identifier: GPL-3.0-or-later

package hostsocket

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/discovery/sd/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type discoverySim struct {
	mock                 *mockLocalListenersExec
	wantDoneBeforeCancel bool
	wantTargetGroups     []model.TargetGroup
}

func (sim *discoverySim) run(t *testing.T) {
	d, err := NewNetSocketDiscoverer(NetworkSocketConfig{Tags: "hostsocket net"})
	require.NoError(t, err)

	d.ll = sim.mock

	ctx, cancel := context.WithCancel(context.Background())
	tggs, done := sim.collectTargetGroups(t, ctx, d)

	if sim.wantDoneBeforeCancel {
		select {
		case <-done:
		default:
			assert.Fail(t, "discovery hasn't finished before cancel")
		}
	}
	assert.Equal(t, sim.wantTargetGroups, tggs)

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second * 3):
		assert.Fail(t, "discovery hasn't finished after cancel")
	}
}

func (sim *discoverySim) collectTargetGroups(t *testing.T, ctx context.Context, d *NetDiscoverer) ([]model.TargetGroup, chan struct{}) {

	in := make(chan []model.TargetGroup)
	done := make(chan struct{})

	go func() { defer close(done); d.Discover(ctx, in) }()

	timeout := time.Second * 5
	var tggs []model.TargetGroup

	func() {
		for {
			select {
			case groups := <-in:
				if tggs = append(tggs, groups...); len(tggs) == len(sim.wantTargetGroups) {
					return
				}
			case <-done:
				return
			case <-time.After(timeout):
				t.Logf("discovery timed out after %s", timeout)
				return
			}
		}
	}()

	return tggs, done
}
