// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"context"
	"errors"
	"net/netip"
	"testing"

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

func TestRuntimeEngine_DiscoverByCIDRs_InvalidRequest(t *testing.T) {
	eng, err := NewRuntimeEngine(&fakeObservationProvider{})
	require.NoError(t, err)

	_, err = eng.DiscoverByCIDRs(context.Background(), CIDRRequest{})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidRequest)
}

func TestRuntimeEngine_DiscoverByDevices_InvalidRequest(t *testing.T) {
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

	result, err := eng.DiscoverByCIDRs(context.Background(), CIDRRequest{
		CIDRs: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/24")},
	})
	require.NoError(t, err)
	require.Empty(t, result.Devices)
	require.Empty(t, result.Adjacencies)
	require.Equal(t, 0, result.Stats["links_total"])
}

func TestRuntimeEngine_DiscoverByDevices_NilProvider(t *testing.T) {
	eng := &RuntimeEngine{}
	_, err := eng.DiscoverByDevices(context.Background(), DeviceRequest{
		Devices: []DeviceTarget{{Address: netip.MustParseAddr("10.0.0.1")}},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNotImplemented)
}
