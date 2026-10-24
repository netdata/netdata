// SPDX-License-Identifier: GPL-3.0-or-later

package bmc

import (
	"testing"

	"github.com/bougou/go-ipmi/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestConvertReading(t *testing.T) {
	tenths := types.ReadingFactors{
		M:     1,
		R_Exp: -1,
	}

	tests := map[string]struct {
		raw           byte
		format        types.SensorAnalogUnitFormat
		factors       types.ReadingFactors
		linearization types.LinearizationFunc
		want          float64
		wantOK        bool
	}{
		"unsigned": {
			raw:     25,
			format:  types.SensorAnalogUnitFormat_Unsigned,
			factors: tenths,
			want:    2.5,
			wantOK:  true,
		},
		"1's complement": {
			raw:     0xe6,
			format:  types.SensorAnalogUnitFormat_1sComplement,
			factors: tenths,
			want:    -2.5,
			wantOK:  true,
		},
		"2's complement": {
			raw:     0xe7,
			format:  types.SensorAnalogUnitFormat_2sComplement,
			factors: tenths,
			want:    -2.5,
			wantOK:  true,
		},
		"EXP10 of a negative fraction": {
			raw:           0xf1, // -15
			format:        types.SensorAnalogUnitFormat_2sComplement,
			factors:       tenths,
			linearization: types.LinearizationFunc_EXP10,
			want:          0.03162277660168379, // 10^-1.5
			wantOK:        true,
		},
		"not analog": {
			raw:     25,
			format:  types.SensorAnalogUnitFormat_NotAnalog,
			factors: tenths,
		},
		"LN of zero is not finite": {
			factors: types.ReadingFactors{
				M: 511,
			},
			linearization: types.LinearizationFunc_LN,
		},
		"1/x of zero is not finite": {
			factors: types.ReadingFactors{
				M: 511,
			},
			linearization: types.LinearizationFunc_1X,
		},
		"EXP10 overflow is not finite": {
			raw: 255,
			factors: types.ReadingFactors{
				M: 511,
			},
			linearization: types.LinearizationFunc_EXP10,
		},
		"OEM non-linear": {
			factors: types.ReadingFactors{
				M: 511,
			},
			linearization: 0x71,
		},
		"reserved linearization": {
			factors: types.ReadingFactors{
				M: 511,
			},
			linearization: 0x0c,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := convertReading(
				tc.raw,
				types.SensorUnit{
					AnalogDataFormat: tc.format,
				},
				tc.factors,
				tc.linearization,
			)
			assert.Equal(t, tc.wantOK, ok)
			assert.InDelta(t, tc.want, got, 1e-14)
		})
	}
}
