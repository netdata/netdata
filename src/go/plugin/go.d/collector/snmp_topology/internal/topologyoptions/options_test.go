// SPDX-License-Identifier: GPL-3.0-or-later

package topologyoptions

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseDepth(t *testing.T) {
	tests := map[string]struct {
		in   string
		want int
	}{
		"empty":       {want: DepthAllInternal},
		"all":         {in: "all", want: DepthAllInternal},
		"invalid":     {in: "invalid", want: DepthAllInternal},
		"below-min":   {in: "-2", want: DepthMin},
		"min":         {in: "0", want: DepthMin},
		"within":      {in: "3", want: 3},
		"above-max":   {in: "99", want: DepthMax},
		"surrounded":  {in: " 2 ", want: 2},
		"case-folded": {in: "ALL", want: DepthAllInternal},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, ParseDepth(tc.in))
		})
	}
}

func TestNormalizeQueryOptions(t *testing.T) {
	tests := map[string]struct {
		in   QueryOptions
		want QueryOptions
	}{
		"defaults": {
			want: QueryOptions{
				MapType:            MapTypeLLDPCDPManaged,
				InferenceStrategy:  InferenceStrategyFDBMinimumKnowledge,
				ManagedDeviceFocus: ManagedFocusAllDevices,
				Depth:              0,
			},
		},
		"normalizes-values": {
			in: QueryOptions{
				MapType:            "HIGH_CONFIDENCE_INFERRED",
				InferenceStrategy:  "STP_FDB_CORRELATED",
				ManagedDeviceFocus: "ip: ::ffff:10.0.0.2, ip:10.0.0.1",
				Depth:              99,
			},
			want: QueryOptions{
				MapType:            MapTypeHighConfidenceInferred,
				InferenceStrategy:  InferenceStrategySTPFDBCorrelated,
				ManagedDeviceFocus: "ip:10.0.0.1,ip:10.0.0.2",
				Depth:              DepthMax,
			},
		},
		"keeps-all-depth": {
			in: QueryOptions{
				MapType:            MapTypeAllDevicesLowConfidence,
				InferenceStrategy:  InferenceStrategyCDPFDBHybrid,
				ManagedDeviceFocus: ManagedFocusAllDevices,
				Depth:              DepthAllInternal,
			},
			want: QueryOptions{
				MapType:            MapTypeAllDevicesLowConfidence,
				InferenceStrategy:  InferenceStrategyCDPFDBHybrid,
				ManagedDeviceFocus: ManagedFocusAllDevices,
				Depth:              DepthAllInternal,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, NormalizeQueryOptions(tc.in))
		})
	}
}
