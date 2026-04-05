// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"errors"
	"strconv"
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

func TestTopologyUpdaterFirstRunFailureAndRecovery(t *testing.T) {
	var calls int
	boom := errors.New("boom")

	updater := NewTopologyUpdater(func() (NetworkRouterTopology, error) {
		calls++
		if calls == 1 {
			return NetworkRouterTopology{}, boom
		}
		return NetworkRouterTopology{
			Vertices:      []NetworkRouterVertex{{ID: "node-1", Label: "node-1"}},
			DefaultVertex: "node-1",
		}, nil
	}, nil, nil)

	require.False(t, updater.HasRun())
	require.Nil(t, updater.LastError())

	updater.RunSchedulable()

	require.False(t, updater.HasRun())
	require.ErrorIs(t, updater.LastError(), boom)
	require.Empty(t, updater.GetTopology().Vertices)

	updater.RunSchedulable()

	require.True(t, updater.HasRun())
	require.NoError(t, updater.LastError())
	topology := updater.GetTopology()
	require.Equal(t, "node-1", topology.DefaultVertex)
	require.Len(t, topology.Vertices, 1)

	topology.Vertices[0].ID = "mutated"
	require.Equal(t, "node-1", updater.GetTopology().Vertices[0].ID)
}

func TestTopologyUpdaterForceRunAndParseUpdatesTriggerRefresh(t *testing.T) {
	var (
		builderCalls int
		parseCalls   int
		refreshCalls int
		parseUpdates bool
	)

	updater := NewTopologyUpdater(
		func() (NetworkRouterTopology, error) {
			builderCalls++
			vertexID := strconv.Itoa(builderCalls)
			return NetworkRouterTopology{
				Vertices:      []NetworkRouterVertex{{ID: vertexID, Label: "node-" + vertexID}},
				DefaultVertex: vertexID,
			}, nil
		},
		func() bool {
			parseCalls++
			return parseUpdates
		},
		func() {
			refreshCalls++
		},
	)

	updater.RunSchedulable()
	require.True(t, updater.HasRun())
	require.Equal(t, 1, builderCalls)
	require.Equal(t, 0, parseCalls)
	require.Equal(t, 0, refreshCalls)
	require.Equal(t, "1", updater.GetTopology().DefaultVertex)

	updater.RunSchedulable()
	require.Equal(t, 1, builderCalls)
	require.Equal(t, 1, parseCalls)
	require.Equal(t, 0, refreshCalls)
	require.Equal(t, "1", updater.GetTopology().DefaultVertex)

	updater.ForceRun()
	updater.RunSchedulable()
	require.Equal(t, 2, builderCalls)
	require.Equal(t, 2, parseCalls)
	require.Equal(t, 1, refreshCalls)
	require.Equal(t, "2", updater.GetTopology().DefaultVertex)

	parseUpdates = true
	updater.RunSchedulable()
	require.Equal(t, 3, builderCalls)
	require.Equal(t, 3, parseCalls)
	require.Equal(t, 2, refreshCalls)
	require.Equal(t, "3", updater.GetTopology().DefaultVertex)
	require.NoError(t, updater.LastError())
}

func TestTopologyUpdaterRefreshFailureKeepsLastSuccessfulTopology(t *testing.T) {
	var (
		builderCalls int
		parseUpdates bool
	)
	boom := errors.New("refresh failed")

	updater := NewTopologyUpdater(
		func() (NetworkRouterTopology, error) {
			builderCalls++
			if builderCalls == 2 {
				return NetworkRouterTopology{}, boom
			}
			vertexID := strconv.Itoa(builderCalls)
			return NetworkRouterTopology{
				Vertices:      []NetworkRouterVertex{{ID: vertexID, Label: "node-" + vertexID}},
				DefaultVertex: vertexID,
			}, nil
		},
		func() bool {
			return parseUpdates
		},
		nil,
	)

	updater.RunSchedulable()
	require.True(t, updater.HasRun())
	require.Equal(t, "1", updater.GetTopology().DefaultVertex)
	require.NoError(t, updater.LastError())

	parseUpdates = true
	updater.RunSchedulable()
	require.True(t, updater.HasRun())
	require.ErrorIs(t, updater.LastError(), boom)
	require.Equal(t, "1", updater.GetTopology().DefaultVertex)
}
