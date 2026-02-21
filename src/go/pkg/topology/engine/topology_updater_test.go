// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Port of TopologyUpdaterIT.verifyGetTopologyAccessWhileDiscoveryInProgressDoesNotBlock.
func TestTopologyUpdater_GetTopologyDoesNotBlockDuringDiscovery(t *testing.T) {
	var buildCalls atomic.Int64
	updater := NewTopologyUpdater(func() (NetworkRouterTopology, error) {
		buildCalls.Add(1)
		time.Sleep(1 * time.Second)
		return NetworkRouterTopology{
			Vertices: []NetworkRouterVertex{
				{ID: "v1", Label: "Vertex 1"},
				{ID: "v2", Label: "Vertex 2"},
			},
		}, nil
	}, nil, nil)

	discoveryDone := make(chan struct{})
	go func() {
		updater.RunSchedulable()
		close(discoveryDone)
	}()

	type topoResult struct {
		vertices int
		edges    int
	}

	getCurrent := func() <-chan topoResult {
		done := make(chan topoResult, 1)
		go func() {
			current := updater.GetTopology()
			done <- topoResult{
				vertices: len(current.Vertices),
				edges:    len(current.Edges),
			}
		}()
		return done
	}

	select {
	case <-time.After(1 * time.Second):
		t.Fatalf("get topology timed out while discovery was running")
	case result := <-getCurrent():
		require.Equal(t, 0, result.vertices)
		require.Equal(t, 0, result.edges)
	}

	select {
	case <-time.After(3 * time.Second):
		t.Fatalf("discovery did not complete")
	case <-discoveryDone:
	}

	select {
	case <-time.After(1 * time.Second):
		t.Fatalf("get topology timed out after discovery")
	case result := <-getCurrent():
		require.Equal(t, 2, result.vertices)
		require.Equal(t, 0, result.edges)
	}

	require.Equal(t, int64(1), buildCalls.Load())
}
