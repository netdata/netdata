// SPDX-License-Identifier: GPL-3.0-or-later

package redfishsd

import (
	"context"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"
	"github.com/stretchr/testify/require"
)

const asyncTestTimeout = 10 * time.Second

func TestScanUsesPositiveCacheAndRemovesFailedReprobe(t *testing.T) {
	t.Setenv("NETDATA_LIB_DIR", t.TempDir())
	cfg := decodeDiscoveryConfig(t, `{
		"device_cache_ttl":"1s",
		"profiles":[{"name":"p","scheme":"http","auth_method":"none"}],
		"networks":[{"subnet":"192.0.2.1","ports":[80],"profile":"p"}]
	}`)
	discoverer, err := NewDiscoverer(cfg)
	require.NoError(t, err)
	now := time.Unix(100, 0).UTC()
	discoverer.now = func() time.Time { return now }
	var calls atomic.Int64
	discoverer.probe = func(context.Context, endpointCandidate) error {
		calls.Add(1)
		return nil
	}

	output := make(chan []model.TargetGroup, 1)
	discoverer.scan(context.Background(), output)
	groups := <-output
	require.Len(t, groups, 1)
	require.Len(t, groups[0].Targets(), 1)
	require.EqualValues(t, 1, calls.Load())

	now = now.Add(500 * time.Millisecond)
	discoverer.scan(context.Background(), output)
	groups = <-output
	require.Len(t, groups[0].Targets(), 1)
	require.EqualValues(t, 1, calls.Load())

	now = now.Add(2 * time.Second)
	discoverer.probe = func(context.Context, endpointCandidate) error {
		calls.Add(1)
		return context.DeadlineExceeded
	}
	discoverer.scan(context.Background(), output)
	groups = <-output
	require.Empty(t, groups[0].Targets())
	require.EqualValues(t, 2, calls.Load())
	require.Empty(t, discoverer.status.Endpoints)
}

func TestCanceledScanDoesNotPruneStatusOrPublishPartialOutput(t *testing.T) {
	t.Setenv("NETDATA_LIB_DIR", t.TempDir())
	cfg := decodeDiscoveryConfig(t, `{
		"profiles":[{"name":"p","scheme":"http","auth_method":"none"}],
		"networks":[{"subnet":"192.0.2.1","ports":[80,8080],"profile":"p"}]
	}`)
	discoverer, err := NewDiscoverer(cfg)
	require.NoError(t, err)
	discoverer.status.Endpoints["http://192.0.2.1:8080/redfish/v1/"] = statusEntry{
		ValidatedAt: time.Unix(100, 0).UTC(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancel()
	output := make(chan []model.TargetGroup, 1)
	discoverer.scan(ctx, output)

	require.Contains(t, discoverer.status.Endpoints, "http://192.0.2.1:8080/redfish/v1/")
	select {
	case <-output:
		t.Fatal("canceled discovery scan published partial output")
	default:
	}
}

func TestCanceledInflightProbePreservesCachedStatus(t *testing.T) {
	t.Setenv("NETDATA_LIB_DIR", t.TempDir())
	cfg := decodeDiscoveryConfig(t, `{
		"device_cache_ttl":"1s",
		"profiles":[{"name":"p","scheme":"http","auth_method":"none"}],
		"networks":[{"subnet":"192.0.2.1","ports":[80],"profile":"p"}]
	}`)
	discoverer, err := NewDiscoverer(cfg)
	require.NoError(t, err)
	discoverer.now = func() time.Time { return time.Unix(100, 0).UTC() }
	const origin = "http://192.0.2.1/redfish/v1/"
	discoverer.status.Endpoints[origin] = statusEntry{ValidatedAt: time.Unix(1, 0).UTC()}

	started := make(chan struct{})
	discoverer.probe = func(ctx context.Context, _ endpointCandidate) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	output := make(chan []model.TargetGroup, 1)
	done := make(chan struct{})
	go func() {
		discoverer.scan(ctx, output)
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(asyncTestTimeout):
		cancel()
		t.Fatal("timed out waiting for Redfish discovery probe to start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(asyncTestTimeout):
		t.Fatal("canceled discovery scan did not finish")
	}

	require.Contains(t, discoverer.status.Endpoints, origin)
	select {
	case <-output:
		t.Fatal("canceled discovery scan published partial output")
	default:
	}
}

func TestTargetIdentityDoesNotChangeWithProfileSettings(t *testing.T) {
	candidate, err := makeCandidate(mustAddr(t, "192.0.2.5"), 443, ProfileConfig{
		Name: "one", Scheme: "https", JobConfig: map[string]any{
			"username": "a",
			"password": "first",
		},
		hashRevision: 1,
	})
	require.NoError(t, err)
	first := newTarget(candidate)
	candidate.profile.Name = "two"
	candidate.profile.JobConfig = map[string]any{
		"username": "b",
		"password": "second",
	}
	second := newTarget(candidate)
	require.Equal(t, first.TUID(), second.TUID())
	require.NotEqual(t, first.Hash(), second.Hash())

	candidate.profile.Name = "one"
	sameRevision := newTarget(candidate)
	require.Equal(t, first.Hash(), sameRevision.Hash())

	candidate.profile.hashRevision++
	newRevision := newTarget(candidate)
	require.NotEqual(t, sameRevision.Hash(), newRevision.Hash())
}

func TestScanEnforcesOnePipelineWideConcurrencyLimit(t *testing.T) {
	t.Setenv("NETDATA_LIB_DIR", t.TempDir())
	cfg := decodeDiscoveryConfig(t, `{
		"max_concurrent_scans":2,
		"profiles":[{"name":"p","scheme":"http","auth_method":"none"}],
		"networks":[
			{"subnet":"192.0.2.1/32","ports":[80,8080],"profile":"p"},
			{"subnet":"192.0.2.2/32","ports":[80,8080],"profile":"p"}
		]
	}`)
	discoverer, err := NewDiscoverer(cfg)
	require.NoError(t, err)
	var active, maximum atomic.Int64
	release := make(chan struct{})
	discoverer.probe = func(ctx context.Context, _ endpointCandidate) error {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			previous := maximum.Load()
			if current <= previous || maximum.CompareAndSwap(previous, current) {
				break
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-release:
			return nil
		}
	}

	output := make(chan []model.TargetGroup, 1)
	done := make(chan struct{})
	go func() {
		discoverer.scan(context.Background(), output)
		close(done)
	}()
	require.Eventually(t, func() bool { return maximum.Load() == 2 }, asyncTestTimeout, time.Millisecond)
	close(release)
	select {
	case <-done:
	case <-time.After(asyncTestTimeout):
		t.Fatal("discovery scan did not finish")
	}
	var groups []model.TargetGroup
	select {
	case groups = <-output:
	default:
		t.Fatal("completed discovery scan did not publish output")
	}
	require.Len(t, groups, 1)
	require.Len(t, groups[0].Targets(), 4)
	require.EqualValues(t, 2, maximum.Load())
}

func mustAddr(t *testing.T, value string) (result netip.Addr) {
	t.Helper()
	result, err := netip.ParseAddr(value)
	require.NoError(t, err)
	return result
}
