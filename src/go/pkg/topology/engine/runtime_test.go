// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeObservationProvider struct {
	cidrObs    []L2Observation
	deviceObs  []L2Observation
	cidrErr    error
	deviceErr  error
	cidrReqs   []CIDRRequest
	deviceReqs []DeviceRequest
}

func (p *fakeObservationProvider) ObserveByCIDRs(_ context.Context, req CIDRRequest) ([]L2Observation, error) {
	p.cidrReqs = append(p.cidrReqs, req)
	if p.cidrErr != nil {
		return nil, p.cidrErr
	}
	return p.cidrObs, nil
}

func (p *fakeObservationProvider) ObserveByDevices(_ context.Context, req DeviceRequest) ([]L2Observation, error) {
	p.deviceReqs = append(p.deviceReqs, req)
	if p.deviceErr != nil {
		return nil, p.deviceErr
	}
	return p.deviceObs, nil
}

func TestNewRuntimeEngine_RequiresProvider(t *testing.T) {
	_, err := NewRuntimeEngine(nil)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidRequest)
}

func TestRuntimeEngine_DiscoverByDevices_BuildsResult(t *testing.T) {
	provider := &fakeObservationProvider{
		deviceObs: []L2Observation{
			{
				DeviceID:     "switch-a",
				Hostname:     "switch-a.example.net",
				ManagementIP: "10.0.0.1",
				LLDPRemotes: []LLDPRemoteObservation{
					{
						LocalPortNum: "8",
						LocalPortID:  "Gi0/0",
						SysName:      "switch-b.example.net",
						PortID:       "Gi0/1",
					},
				},
			},
			{
				DeviceID: "switch-b",
				Hostname: "switch-b.example.net",
			},
		},
	}
	eng, err := NewRuntimeEngine(provider)
	require.NoError(t, err)

	req := DeviceRequest{
		Devices: []DeviceTarget{{Address: netip.MustParseAddr("10.0.0.1")}},
		Options: DiscoverOptions{EnableLLDP: true},
	}

	result, err := eng.DiscoverByDevices(context.Background(), req)
	require.NoError(t, err)
	require.Len(t, provider.deviceReqs, 1)
	require.Len(t, result.Adjacencies, 1)
	require.Equal(t, "lldp", result.Adjacencies[0].Protocol)
	require.Equal(t, "switch-a", result.Adjacencies[0].SourceID)
	require.Equal(t, "switch-b", result.Adjacencies[0].TargetID)
}

func TestRuntimeEngine_DiscoverByDevices_PropagatesCollectedAt(t *testing.T) {
	sourceZone := time.FixedZone("UTC+02", 2*60*60)
	collectedAt := time.Date(2026, time.April, 2, 3, 4, 5, 0, sourceZone)
	expectedCollectedAt := collectedAt.UTC()
	provider := &fakeObservationProvider{
		deviceObs: []L2Observation{
			{
				DeviceID: "switch-a",
				Hostname: "switch-a.example.net",
			},
		},
	}
	eng, err := NewRuntimeEngine(provider)
	require.NoError(t, err)

	req := DeviceRequest{
		Devices: []DeviceTarget{{Address: netip.MustParseAddr("10.0.0.1")}},
		Options: DiscoverOptions{CollectedAt: collectedAt},
	}

	result, err := eng.DiscoverByDevices(context.Background(), req)
	require.NoError(t, err)
	require.Len(t, provider.deviceReqs, 1)
	require.Equal(t, expectedCollectedAt, provider.deviceReqs[0].Options.CollectedAt)
	require.Equal(t, expectedCollectedAt, result.CollectedAt)
}

func TestRuntimeEngine_DiscoverByCIDRs_InvalidRequest(t *testing.T) {
	eng, err := NewRuntimeEngine(&fakeObservationProvider{})
	require.NoError(t, err)

	_, err = eng.DiscoverByCIDRs(context.Background(), CIDRRequest{})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidRequest)
}

func TestRuntimeEngine_DiscoverByCIDRs_InvalidPrefix(t *testing.T) {
	provider := &fakeObservationProvider{}
	eng, err := NewRuntimeEngine(provider)
	require.NoError(t, err)

	_, err = eng.DiscoverByCIDRs(context.Background(), CIDRRequest{
		CIDRs: []netip.Prefix{{}},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidRequest)
	require.ErrorContains(t, err, "cidrs[0] has invalid prefix")
	require.Empty(t, provider.cidrReqs)
}

func TestRuntimeEngine_DiscoverByDevices_InvalidRequest(t *testing.T) {
	eng, err := NewRuntimeEngine(&fakeObservationProvider{})
	require.NoError(t, err)

	_, err = eng.DiscoverByDevices(context.Background(), DeviceRequest{})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidRequest)
	require.ErrorContains(t, err, "devices are required")
}

func TestRuntimeEngine_DiscoverByDevices_InvalidAddress(t *testing.T) {
	eng, err := NewRuntimeEngine(&fakeObservationProvider{})
	require.NoError(t, err)

	_, err = eng.DiscoverByDevices(context.Background(), DeviceRequest{
		Devices: []DeviceTarget{{Address: netip.Addr{}}},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidRequest)
}

func TestRuntimeEngine_DiscoverByCIDRs_ProviderError(t *testing.T) {
	providerErr := errors.New("provider failed")
	provider := &fakeObservationProvider{cidrErr: providerErr}
	eng, err := NewRuntimeEngine(provider)
	require.NoError(t, err)

	_, err = eng.DiscoverByCIDRs(context.Background(), CIDRRequest{
		CIDRs: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/24")},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, providerErr)
}

func TestRuntimeEngine_EmptyObservationsReturnEmptyResult(t *testing.T) {
	provider := &fakeObservationProvider{}
	eng, err := NewRuntimeEngine(provider)
	require.NoError(t, err)

	sourceZone := time.FixedZone("UTC-03", -3*60*60)
	collectedAt := time.Date(2026, time.April, 2, 3, 4, 5, 0, sourceZone)
	expectedCollectedAt := collectedAt.UTC()

	result, err := eng.DiscoverByCIDRs(context.Background(), CIDRRequest{
		CIDRs:   []netip.Prefix{netip.MustParsePrefix("10.0.0.0/24")},
		Options: DiscoverOptions{CollectedAt: collectedAt},
	})
	require.NoError(t, err)
	require.Len(t, provider.cidrReqs, 1)
	require.Equal(t, expectedCollectedAt, provider.cidrReqs[0].Options.CollectedAt)
	require.Equal(t, expectedCollectedAt, result.CollectedAt)
	require.Empty(t, result.Devices)
	require.Empty(t, result.Adjacencies)
	require.Equal(t, 0, result.Stats["links_total"])
	require.Equal(t, 0, result.Stats["identity_alias_endpoints_mapped"])
	require.Equal(t, 0, result.Stats["identity_alias_endpoints_ambiguous_mac"])
	require.Equal(t, 0, result.Stats["identity_alias_ips_merged"])
	require.Equal(t, 0, result.Stats["identity_alias_ips_conflict_skipped"])
}

func TestRuntimeEngine_EmptyResultStatsSchemaMatchesPipelineResult(t *testing.T) {
	emptyStats := emptyResult(time.Time{}).Stats
	pipelineResult, err := BuildL2ResultFromObservations([]L2Observation{{DeviceID: "switch-a"}}, DiscoverOptions{})
	require.NoError(t, err)

	require.ElementsMatch(t, statsKeys(pipelineResult.Stats), statsKeys(emptyStats))
}

func TestRuntimeEngine_DiscoverByDevices_NilReceiver(t *testing.T) {
	var eng *RuntimeEngine
	_, err := eng.DiscoverByDevices(context.Background(), DeviceRequest{
		Devices: []DeviceTarget{{Address: netip.MustParseAddr("10.0.0.1")}},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidRequest)
}

func TestRuntimeEngine_DiscoverByDevices_NilProvider(t *testing.T) {
	eng := &RuntimeEngine{}
	_, err := eng.DiscoverByDevices(context.Background(), DeviceRequest{
		Devices: []DeviceTarget{{Address: netip.MustParseAddr("10.0.0.1")}},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidRequest)
}

func statsKeys(stats map[string]any) []string {
	keys := make([]string, 0, len(stats))
	for key := range stats {
		keys = append(keys, key)
	}
	return keys
}

func TestRuntimeEngine_DiscoverByCIDRs_NilReceiver(t *testing.T) {
	var eng *RuntimeEngine
	_, err := eng.DiscoverByCIDRs(context.Background(), CIDRRequest{
		CIDRs: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/24")},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidRequest)
}

func TestRuntimeEngine_DiscoverByCIDRs_NilProvider(t *testing.T) {
	eng := &RuntimeEngine{}
	_, err := eng.DiscoverByCIDRs(context.Background(), CIDRRequest{
		CIDRs: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/24")},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidRequest)
}

func TestEnsureCollectedAt_DefaultsZeroToUTCNow(t *testing.T) {
	before := time.Now().UTC()
	opts := ensureCollectedAt(DiscoverOptions{})
	after := time.Now().UTC()

	require.False(t, opts.CollectedAt.IsZero())
	require.Equal(t, time.UTC, opts.CollectedAt.Location())
	require.False(t, opts.CollectedAt.Before(before))
	require.False(t, opts.CollectedAt.After(after))
}
