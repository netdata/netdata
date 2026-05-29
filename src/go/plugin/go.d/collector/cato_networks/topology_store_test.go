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
	var store topologyStore

	_, ok := store.CurrentTopology()
	require.False(t, ok)

	data := &topology.Data{Source: topologySource}
	store.Publish(data)

	got, ok := store.CurrentTopology()
	require.True(t, ok)
	require.Same(t, data, got)
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
	var deps funcDepsAdapter

	data, ok := deps.CurrentTopology()

	require.Nil(t, data)
	require.False(t, ok)
}
