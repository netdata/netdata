// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	"github.com/stretchr/testify/require"
)

func TestFuncDepsAdapterSnapshotUnavailable(t *testing.T) {
	tests := map[string]struct {
		adapter funcDepsAdapter
	}{
		"nil registry": {
			adapter: funcDepsAdapter{},
		},
		"empty registry": {
			adapter: funcDepsAdapter{registry: newTopologyRegistry()},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			data, ok, err := tc.adapter.Snapshot(topologyoptions.QueryOptions{})

			require.NoError(t, err)
			require.False(t, ok)
			require.Equal(t, topologyv1.Data{}, data)
		})
	}
}

func TestFuncDepsAdapterManagedDeviceFocusTargetsNilRegistry(t *testing.T) {
	require.Nil(t, funcDepsAdapter{}.ManagedDeviceFocusTargets())
}

func TestFuncDepsAdapterSnapshotUsesCachedReverseDNSWithoutLiveLookup(t *testing.T) {
	clock := newReverseDNSTestClock()
	var liveCalls atomic.Int64
	registry := newTopologyRegistry()
	registry.reverseDNS = newTopologyReverseDNSResolverWithConfig(topologyReverseDNSConfig{
		now: clock.Now,
		lookup: func(context.Context, string) ([]string, error) {
			liveCalls.Add(1)
			return []string{"unexpected.example.test"}, nil
		},
	})
	registry.reverseDNS.store("10.0.0.10", "switch-a.example.test", clock.Now().Add(time.Hour))

	cache := newTopologyCache()
	seedPublishedEndpointSnapshot(cache)
	registry.register(cache)

	data, ok, err := (funcDepsAdapter{registry: registry}).Snapshot(topologyoptions.QueryOptions{})
	require.NoError(t, err)
	require.True(t, ok)
	require.Contains(t, topologyV1StringColumnValues(t, data, data.Actors, "display_name"), "switch-a.example.test")
	require.Zero(t, liveCalls.Load())
}

func TestFuncDepsAdapterSnapshotEnqueuesReverseDNSWarmCandidates(t *testing.T) {
	clock := newReverseDNSTestClock()
	warmed := make(chan string, 4)
	registry := newTopologyRegistry()
	registry.reverseDNS = newTopologyReverseDNSResolverWithConfig(topologyReverseDNSConfig{
		now:         clock.Now,
		timeout:     time.Second,
		positiveTTL: time.Hour,
		negativeTTL: time.Minute,
		maxEntries:  10,
		concurrency: 1,
		lookup: func(_ context.Context, ip string) ([]string, error) {
			warmed <- ip
			return []string{ip + ".example.test"}, nil
		},
	})
	registry.setReverseDNSWarmContext(context.Background())

	cache := newTopologyCache()
	seedPublishedEndpointSnapshot(cache)
	registry.register(cache)

	_, ok, err := (funcDepsAdapter{registry: registry}).Snapshot(topologyoptions.QueryOptions{})
	require.NoError(t, err)
	require.True(t, ok)

	require.Eventually(t, func() bool {
		select {
		case ip := <-warmed:
			return ip == "10.0.0.10" || ip == "10.0.0.20"
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
}
