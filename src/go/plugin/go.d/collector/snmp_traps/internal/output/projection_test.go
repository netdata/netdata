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

func TestCanonicalizeValueUsesStringFallback(t *testing.T) {
	got := CanonicalizeValue(struct{ Value string }{Value: "opaque"})
	assert.Equal(t, ValueString, got.Kind)
	assert.Equal(t, "{opaque}", got.String)
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
