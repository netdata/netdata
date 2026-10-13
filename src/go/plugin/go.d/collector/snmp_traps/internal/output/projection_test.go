// SPDX-License-Identifier: GPL-3.0-or-later

package output

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSortedLabelKeysReusesDestination(t *testing.T) {
	dst := make([]string, 0, 3)
	got := SortedLabelKeys(dst, map[string]string{"z": "3", "a": "1", "m": "2"})
	assert.Equal(t, []string{"a", "m", "z"}, got)
	assert.Equal(t, cap(dst), cap(got))
}

func TestCanonicalizeValue(t *testing.T) {
	tests := map[string]struct {
		value any
		want  CanonicalValue
	}{
		"nil":      {want: CanonicalValue{Kind: ValueNull}},
		"string":   {value: "text", want: CanonicalValue{Kind: ValueString, String: "text"}},
		"int64":    {value: int64(-42), want: CanonicalValue{Kind: ValueInt64, Int64: -42}},
		"uint64":   {value: uint64(42), want: CanonicalValue{Kind: ValueUint64, Uint64: 42}},
		"float64":  {value: 1.25, want: CanonicalValue{Kind: ValueFloat64, Float64: 1.25}},
		"bool":     {value: true, want: CanonicalValue{Kind: ValueBool, Bool: true}},
		"bytes":    {value: []byte{0x00, 0x0f, 0xff}, want: CanonicalValue{Kind: ValueBytes, Bytes: []byte{0x00, 0x0f, 0xff}}},
		"fallback": {value: struct{ Value string }{Value: "opaque"}, want: CanonicalValue{Kind: ValueString, String: "{opaque}"}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, CanonicalizeValue(tc.value))
		})
	}
}

func TestVarbindProjector(t *testing.T) {
	var projector VarbindProjector
	projector.Reset(4)

	first, ok := projector.Project(model.VarbindValue{Name: "ifName", OID: ".1", Type: "OctetString", Value: "eth0"})
	require.True(t, ok)
	assert.Equal(t, "ifName", first.Key)
	assert.Equal(t, ValueString, first.Value.Kind)
	assert.Equal(t, "eth0", first.Value.String)

	second, ok := projector.Project(model.VarbindValue{Name: "ifName", OID: ".2", Type: "OctetString", Value: []byte{0xab, 0xcd}})
	require.True(t, ok)
	assert.Equal(t, "ifName#2", second.Key)
	assert.Equal(t, ValueBytes, second.Value.Kind)
	assert.Equal(t, []byte{0xab, 0xcd}, second.Value.Bytes)

	_, ok = projector.Project(model.VarbindValue{Name: "snmpTrapCommunity.0", OID: model.SNMPTrapCommunityOID, Value: "secret"})
	assert.False(t, ok)

	projector.Reset(1)
	again, ok := projector.Project(model.VarbindValue{Name: "ifName", Value: "eth1"})
	require.True(t, ok)
	assert.Equal(t, "ifName", again.Key)
}
