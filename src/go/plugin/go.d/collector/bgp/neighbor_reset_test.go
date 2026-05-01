// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIDPartPreservesDistinctValues(t *testing.T) {
	assert.NotEqual(t, idPart("vrf-a"), idPart("vrf_a"))
	assert.NotEqual(t, idPart("10.0.0.1"), idPart("10_0_0_1"))
	assert.Equal(t, "vrf__a", idPart("vrf_a"))
	assert.Equal(t, "vrf_x2d_a", idPart("vrf-a"))
}

func TestParseLastErrorCodeSubcode(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantCode    int64
		wantSubcode int64
		wantCodeOK  bool
		wantSubOK   bool
	}{
		{name: "string pair", raw: `"6/3"`, wantCode: 6, wantSubcode: 3, wantCodeOK: true, wantSubOK: true},
		{name: "array pair", raw: `[4,0]`, wantCode: 4, wantSubcode: 0, wantCodeOK: true, wantSubOK: true},
		{name: "object pair", raw: `{"errorCode":5,"errorSubcode":1}`, wantCode: 5, wantSubcode: 1, wantCodeOK: true, wantSubOK: true},
		{name: "single number", raw: `4`, wantCode: 4, wantSubcode: 0, wantCodeOK: true, wantSubOK: false},
		{name: "empty", raw: `""`, wantCode: 0, wantSubcode: 0, wantCodeOK: false, wantSubOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, subcode, okCode, okSub := parseLastErrorCodeSubcode(json.RawMessage(tc.raw))
			assert.Equal(t, tc.wantCode, code)
			assert.Equal(t, tc.wantSubcode, subcode)
			assert.Equal(t, tc.wantCodeOK, okCode)
			assert.Equal(t, tc.wantSubOK, okSub)
		})
	}
}

func TestParseFRRNeighborsIncludesResetDetails(t *testing.T) {
	detailsByVRF, err := parseFRRNeighbors(dataFRRNeighborsRich)
	require.NoError(t, err)

	details, ok := detailsByVRF["default"]["192.168.0.2"]
	require.True(t, ok)

	assert.True(t, details.Reset.HasState)
	assert.False(t, details.Reset.Never)
	assert.True(t, details.Reset.Hard)
	assert.Equal(t, int64(42), details.Reset.AgeSecs)
	assert.True(t, details.Reset.HasResetCode)
	assert.Equal(t, int64(6), details.Reset.ResetCode)
	assert.True(t, details.Reset.HasErrorCode)
	assert.Equal(t, int64(6), details.Reset.ErrorCode)
	assert.True(t, details.Reset.HasErrorSub)
	assert.Equal(t, int64(3), details.Reset.ErrorSubcode)

	neverDetails, ok := detailsByVRF["default"]["192.168.0.4"]
	require.True(t, ok)
	assert.True(t, neverDetails.Reset.HasState)
	assert.True(t, neverDetails.Reset.Never)
	assert.Equal(t, int64(0), neverDetails.Reset.AgeSecs)
}
