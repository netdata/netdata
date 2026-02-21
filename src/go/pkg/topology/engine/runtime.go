// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"context"
	"fmt"
	"time"
)

// ObservationProvider gathers normalized L2 observations for discovery requests.
type ObservationProvider interface {
	ObserveByCIDRs(ctx context.Context, req CIDRRequest) ([]L2Observation, error)
	ObserveByDevices(ctx context.Context, req DeviceRequest) ([]L2Observation, error)
}

// RuntimeEngine executes discovery using a concrete observation provider.
type RuntimeEngine struct {
	provider ObservationProvider
}

// NewRuntimeEngine constructs a concrete engine backed by the given provider.
func NewRuntimeEngine(provider ObservationProvider) (*RuntimeEngine, error) {
	if provider == nil {
		return nil, fmt.Errorf("%w: observation provider is required", ErrInvalidRequest)
	}
	return &RuntimeEngine{provider: provider}, nil
}

func (e *RuntimeEngine) DiscoverByCIDRs(ctx context.Context, req CIDRRequest) (Result, error) {
	if e == nil || e.provider == nil {
		return Result{}, fmt.Errorf("%w: observation provider is not configured", ErrNotImplemented)
	}
	if err := validateCIDRRequest(req); err != nil {
		return Result{}, err
	}

	observations, err := e.provider.ObserveByCIDRs(ctx, req)
	if err != nil {
		return Result{}, fmt.Errorf("observe cidr discovery request: %w", err)
	}
	if len(observations) == 0 {
		return emptyResult(), nil
	}

	result, err := BuildL2ResultFromObservations(observations, req.Options)
	if err != nil {
		return Result{}, fmt.Errorf("build l2 result from cidr discovery request: %w", err)
	}
	return result, nil
}

func (e *RuntimeEngine) DiscoverByDevices(ctx context.Context, req DeviceRequest) (Result, error) {
	if e == nil || e.provider == nil {
		return Result{}, fmt.Errorf("%w: observation provider is not configured", ErrNotImplemented)
	}
	if err := validateDeviceRequest(req); err != nil {
		return Result{}, err
	}

	observations, err := e.provider.ObserveByDevices(ctx, req)
	if err != nil {
		return Result{}, fmt.Errorf("observe device discovery request: %w", err)
	}
	if len(observations) == 0 {
		return emptyResult(), nil
	}

	result, err := BuildL2ResultFromObservations(observations, req.Options)
	if err != nil {
		return Result{}, fmt.Errorf("build l2 result from device discovery request: %w", err)
	}
	return result, nil
}

func validateCIDRRequest(req CIDRRequest) error {
	if len(req.CIDRs) == 0 {
		return fmt.Errorf("%w: cidrs are required", ErrInvalidRequest)
	}
	return nil
}

func validateDeviceRequest(req DeviceRequest) error {
	if len(req.Devices) == 0 {
		return fmt.Errorf("%w: devices are required", ErrInvalidRequest)
	}
	for i := range req.Devices {
		if !req.Devices[i].Address.IsValid() {
			return fmt.Errorf("%w: devices[%d] has invalid address", ErrInvalidRequest, i)
		}
	}
	return nil
}

func emptyResult() Result {
	return Result{
		CollectedAt: time.Now().UTC(),
		Stats: map[string]any{
			"devices_total":        0,
			"links_total":          0,
			"links_lldp":           0,
			"links_cdp":            0,
			"attachments_total":    0,
			"attachments_fdb":      0,
			"enrichments_total":    0,
			"enrichments_arp_nd":   0,
			"bridge_domains_total": 0,
			"endpoints_total":      0,
		},
	}
}
