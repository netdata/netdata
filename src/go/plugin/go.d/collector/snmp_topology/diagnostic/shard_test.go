// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostic

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateShardSequence(t *testing.T) {
	t.Parallel()

	valid := []ShardGeometryV1{
		{CaptureID: 7, Registration: 3, Section: "topology_metrics", Phase: 2, Profile: 4, Shard: 0, ShardCount: 2, FirstOrdinal: 0, RecordCount: 3},
		{CaptureID: 7, Registration: 3, Section: "topology_metrics", Phase: 2, Profile: 4, Shard: 1, ShardCount: 2, FirstOrdinal: 3, RecordCount: 2},
	}
	require.NoError(t, ValidateShardSequence(valid, 5))

	tests := map[string]struct {
		mutate  func([]ShardGeometryV1) []ShardGeometryV1
		expect  uint64
		wantErr string
	}{
		"gap": {
			mutate: func(v []ShardGeometryV1) []ShardGeometryV1 { v[1].FirstOrdinal = 4; return v },
			expect: 5, wantErr: "starts at 4, expected 3",
		},
		"overlap": {
			mutate: func(v []ShardGeometryV1) []ShardGeometryV1 { v[1].FirstOrdinal = 2; return v },
			expect: 5, wantErr: "starts at 2, expected 3",
		},
		"coordinate change": {
			mutate: func(v []ShardGeometryV1) []ShardGeometryV1 { v[1].Profile = 5; return v },
			expect: 5, wantErr: "changes its owner or semantic coordinates",
		},
		"wrong total": {
			mutate: func(v []ShardGeometryV1) []ShardGeometryV1 { return v },
			expect: 6, wantErr: "cover 5 records, expected 6",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := append([]ShardGeometryV1(nil), valid...)
			err := ValidateShardSequence(tc.mutate(got), tc.expect)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestShardGeometryV1_RejectsOrdinalOverflow(t *testing.T) {
	t.Parallel()

	err := (ShardGeometryV1{
		CaptureID:    7,
		Registration: 3,
		Section:      "topology_metrics",
		Phase:        1,
		ShardCount:   1,
		FirstOrdinal: math.MaxUint64,
		RecordCount:  1,
	}).Validate()
	require.ErrorContains(t, err, "arithmetic overflow")
}

func TestValidateShardSequence_Empty(t *testing.T) {
	t.Parallel()

	require.NoError(t, ValidateShardSequence(nil, 0))
	require.ErrorContains(t, ValidateShardSequence([]ShardGeometryV1{{}}, 0), "zero-record section")
}
