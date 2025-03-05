// SPDX-License-Identifier: GPL-3.0-or-later

package dockersd

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/model"

	"github.com/docker/docker/api/types"
	typesContainer "github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type dockerCli interface {
	addContainer(cntr types.Container)
	removeContainer(id string)
}

type discoverySim struct {
	dockerCli  func(cli dockerCli, interval time.Duration)
	wantGroups []model.TargetGroup
}

func (sim *discoverySim) run(t *testing.T) {
	d, err := NewDiscoverer(Config{
		Source: "",
		Tags:   "docker",
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

	wg.Add(1)
	go func() {
		defer wg.Done()
		d.Discover(ctx, in)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
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
	}()

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

	assert.True(t, mock.negApiVerCalled, "NegotiateAPIVersion called")
	assert.True(t, mock.closeCalled, "Close called")
}

func newMockDockerd() *mockDockerd {
	return &mockDockerd{
		containers: make(map[string]types.Container),
	}
}

type mockDockerd struct {
	negApiVerCalled bool
	closeCalled     bool
	mux             sync.Mutex
	containers      map[string]types.Container
}

func (m *mockDockerd) addContainer(cntr types.Container) {
	m.mux.Lock()
	defer m.mux.Unlock()

	m.containers[cntr.ID] = cntr
}

func (m *mockDockerd) removeContainer(id string) {
	m.mux.Lock()
	defer m.mux.Unlock()

	delete(m.containers, id)
}

func (m *mockDockerd) ContainerList(_ context.Context, _ typesContainer.ListOptions) ([]types.Container, error) {
	m.mux.Lock()
	defer m.mux.Unlock()

	var cntrs []types.Container
	for _, cntr := range m.containers {
		cntrs = append(cntrs, cntr)
	}

	return cntrs, nil
}

func (m *mockDockerd) NegotiateAPIVersion(_ context.Context) {
	m.negApiVerCalled = true
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
