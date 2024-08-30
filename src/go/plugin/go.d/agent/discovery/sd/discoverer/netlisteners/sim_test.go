// SPDX-License-Identifier: GPL-3.0-or-later

package netlisteners

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type listenersCli interface {
	addListener(s string)
	removeListener(s string)
}

type discoverySim struct {
	listenersCli func(cli listenersCli, interval, expiry time.Duration)
	wantGroups   []model.TargetGroup
}

func (sim *discoverySim) run(t *testing.T) {
	d, err := NewDiscoverer(Config{
		Source: "",
		Tags:   "netlisteners",
	})
	require.NoError(t, err)

	mock := newMockLocalListenersExec()

	d.ll = mock

	d.interval = time.Millisecond * 100
	d.expiryTime = time.Second * 1

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

	sim.listenersCli(mock, d.interval, d.expiryTime)

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

	wantLen, gotLen := calcTargets(sim.wantGroups), calcTargets(tggs)
	assert.Equalf(t, wantLen, gotLen, "different len (want %d got %d)", wantLen, gotLen)
	assert.Equal(t, sim.wantGroups, tggs)
}

func newMockLocalListenersExec() *mockLocalListenersExec {
	return &mockLocalListenersExec{}
}

type mockLocalListenersExec struct {
	errResponse bool
	mux         sync.Mutex
	listeners   []string
}

func (m *mockLocalListenersExec) addListener(s string) {
	m.mux.Lock()
	defer m.mux.Unlock()

	m.listeners = append(m.listeners, s)
}

func (m *mockLocalListenersExec) removeListener(s string) {
	m.mux.Lock()
	defer m.mux.Unlock()

	if i := slices.Index(m.listeners, s); i != -1 {
		m.listeners = append(m.listeners[:i], m.listeners[i+1:]...)
	}
}

func (m *mockLocalListenersExec) discover(context.Context) ([]byte, error) {
	if m.errResponse {
		return nil, errors.New("mock discover() error")
	}

	m.mux.Lock()
	defer m.mux.Unlock()

	var buf strings.Builder
	for _, s := range m.listeners {
		buf.WriteString(s)
		buf.WriteByte('\n')
	}

	return []byte(buf.String()), nil
}

func calcTargets(tggs []model.TargetGroup) int {
	var n int
	for _, tgg := range tggs {
		n += len(tgg.Targets())
	}
	return n
}

func sortTargetGroups(tggs []model.TargetGroup) {
	if len(tggs) == 0 {
		return
	}
	sort.Slice(tggs, func(i, j int) bool { return tggs[i].Source() < tggs[j].Source() })

	for idx := range tggs {
		tgts := tggs[idx].Targets()
		sort.Slice(tgts, func(i, j int) bool { return tgts[i].Hash() < tgts[j].Hash() })
	}
}
