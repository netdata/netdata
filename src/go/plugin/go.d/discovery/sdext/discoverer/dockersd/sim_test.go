// SPDX-License-Identifier: GPL-3.0-or-later

package dockersd

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"

	typesContainer "github.com/moby/moby/api/types/container"
	docker "github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type dockerCli interface {
	addContainer(cntr typesContainer.Summary)
	removeContainer(id string)
}

type discoverySim struct {
	dockerCli  func(cli dockerCli, interval time.Duration)
	wantGroups []model.TargetGroup
}

func (sim *discoverySim) run(t *testing.T) {
	d, err := NewDiscoverer(Config{
		Source: "",
	})
	require.NoError(t, err)

	mock := newMockDockerd()

	d.newDockerClient = func(addr string) (dockerClient, error) {
		return mock, nil
	}
	d.listInterval = time.Millisecond * 100

	seen := make(map[string]model.TargetGroup)
	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan []model.TargetGroup)
	var wg sync.WaitGroup

	wg.Go(func() {
		d.Discover(ctx, in)
	})

	wg.Go(func() {
		for {
			select {
			case <-ctx.Done():
				return
			case tggs := <-in:
				for _, tgg := range tggs {
					seen[tgg.Source()] = tgg
				}
			}
		}
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()

	select {
	case <-d.started:
	case <-time.After(time.Second * 3):
		require.Fail(t, "discovery failed to start")
	}

	sim.dockerCli(mock, d.listInterval)
	time.Sleep(time.Second)

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second * 3):
		require.Fail(t, "discovery hasn't finished after cancel")
	}

	var tggs []model.TargetGroup
	for _, tgg := range seen {
		tggs = append(tggs, tgg)
	}

	sortTargetGroups(tggs)
	sortTargetGroups(sim.wantGroups)

	wantLen, gotLen := len(sim.wantGroups), len(tggs)
	assert.Equalf(t, wantLen, gotLen, "different len (want %d got %d)", wantLen, gotLen)
	assert.Equal(t, sim.wantGroups, tggs)

	assert.True(t, mock.closeCalled, "Close called")
}

func newMockDockerd() *mockDockerd {
	return &mockDockerd{
		containers: make(map[string]typesContainer.Summary),
	}
}

type mockDockerd struct {
	closeCalled bool
	mux         sync.Mutex
	containers  map[string]typesContainer.Summary
}

func (m *mockDockerd) addContainer(cntr typesContainer.Summary) {
	m.mux.Lock()
	defer m.mux.Unlock()

	m.containers[cntr.ID] = cntr
}

func (m *mockDockerd) removeContainer(id string) {
	m.mux.Lock()
	defer m.mux.Unlock()

	delete(m.containers, id)
}

func (m *mockDockerd) ContainerList(_ context.Context, _ docker.ContainerListOptions) (docker.ContainerListResult, error) {
	m.mux.Lock()
	defer m.mux.Unlock()

	var cntrs []typesContainer.Summary
	for _, cntr := range m.containers {
		cntrs = append(cntrs, cntr)
	}

	return docker.ContainerListResult{Items: cntrs}, nil
}

func (m *mockDockerd) Close() error {
	m.closeCalled = true
	return nil
}

func sortTargetGroups(tggs []model.TargetGroup) {
	if len(tggs) == 0 {
		return
	}
	sort.Slice(tggs, func(i, j int) bool { return tggs[i].Source() < tggs[j].Source() })
}
