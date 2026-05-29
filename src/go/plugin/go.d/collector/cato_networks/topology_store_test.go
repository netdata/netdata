// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
)

func TestTopologyStorePublishesCurrentSnapshot(t *testing.T) {
	tests := map[string]struct {
		setup func(*topologyStore) *topology.Data
		check func(*testing.T, *topologyStore, *topology.Data)
	}{
		"empty store has no topology": {
			check: func(t *testing.T, store *topologyStore, _ *topology.Data) {
				got, ok := store.CurrentTopology()
				require.Nil(t, got)
				require.False(t, ok)
			},
		},
		"load returns published snapshot": {
			setup: func(store *topologyStore) *topology.Data {
				data := &topology.Data{Source: topologySource}
				store.Publish(data)
				return data
			},
			check: func(t *testing.T, store *topologyStore, data *topology.Data) {
				got, ok := store.CurrentTopology()
				require.True(t, ok)
				require.Same(t, data, got)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var store topologyStore
			var data *topology.Data
			if tc.setup != nil {
				data = tc.setup(&store)
			}
			tc.check(t, &store, data)
		})
	}
}

func TestTopologyStoreConcurrentPublishLoad(t *testing.T) {
	var store topologyStore
	var wg sync.WaitGroup
	var nilLoads atomic.Int64

	for i := range 100 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			store.Publish(&topology.Data{Source: fmt.Sprintf("source-%d", i)})
		}(i)
	}

	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if data, ok := store.CurrentTopology(); ok {
				if data == nil {
					nilLoads.Add(1)
				}
			}
		}()
	}

	wg.Wait()

	data, ok := store.CurrentTopology()
	require.True(t, ok)
	require.NotEmpty(t, data.Source)
	require.Zero(t, nilLoads.Load())
}

func TestFuncDepsAdapterWithoutTopologyStore(t *testing.T) {
	tests := map[string]struct {
		deps  funcDepsAdapter
		check func(*testing.T, *topology.Data, bool)
	}{
		"nil store reports unavailable": {
			check: func(t *testing.T, data *topology.Data, ok bool) {
				require.Nil(t, data)
				require.False(t, ok)
			},
		},
		"store delegates current topology": {
			deps: func() funcDepsAdapter {
				var store topologyStore
				store.Publish(&topology.Data{Source: topologySource})
				return funcDepsAdapter{store: &store}
			}(),
			check: func(t *testing.T, data *topology.Data, ok bool) {
				require.True(t, ok)
				require.Equal(t, topologySource, data.Source)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			data, ok := tc.deps.CurrentTopology()
			tc.check(t, data, ok)
		})
	}
}
