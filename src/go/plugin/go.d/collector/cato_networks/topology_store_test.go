// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
)

func TestTopologyStorePublishesCurrentSnapshot(t *testing.T) {
	tests := map[string]struct {
		setup func(*topologyStore) *topologyv1.Data
		check func(*testing.T, *topologyStore, *topologyv1.Data)
	}{
		"empty store has no topology": {
			check: func(t *testing.T, store *topologyStore, _ *topologyv1.Data) {
				got, ok := store.CurrentTopology()
				require.Nil(t, got)
				require.False(t, ok)
			},
		},
		"load returns published snapshot": {
			setup: func(store *topologyStore) *topologyv1.Data {
				data := &topologyv1.Data{Producer: topologyv1.Producer{Source: topologySource}}
				store.Publish(data)
				return data
			},
			check: func(t *testing.T, store *topologyStore, data *topologyv1.Data) {
				got, ok := store.CurrentTopology()
				require.True(t, ok)
				require.Same(t, data, got)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var store topologyStore
			var data *topologyv1.Data
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
			store.Publish(&topologyv1.Data{Producer: topologyv1.Producer{Source: fmt.Sprintf("source-%d", i)}})
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
	require.NotEmpty(t, data.Producer.Source)
	require.Zero(t, nilLoads.Load())
}

func TestFuncDepsAdapterWithoutTopologyStore(t *testing.T) {
	tests := map[string]struct {
		deps  funcDepsAdapter
		check func(*testing.T, *topologyv1.Data, bool)
	}{
		"nil store reports unavailable": {
			check: func(t *testing.T, data *topologyv1.Data, ok bool) {
				require.Nil(t, data)
				require.False(t, ok)
			},
		},
		"store delegates current topology": {
			deps: func() funcDepsAdapter {
				var store topologyStore
				store.Publish(&topologyv1.Data{Producer: topologyv1.Producer{Source: topologySource}})
				return funcDepsAdapter{store: &store}
			}(),
			check: func(t *testing.T, data *topologyv1.Data, ok bool) {
				require.True(t, ok)
				require.Equal(t, topologySource, data.Producer.Source)
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
