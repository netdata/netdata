// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseOpenBGPDRIBSummary(t *testing.T) {
	summary, err := parseOpenBGPDRIBSummary(dataOpenBGPDRIB)
	require.NoError(t, err)

	assert.Equal(t, openbgpdRIBSummary{
		RIBRoutes: 2,
		Valid:     1,
		NotFound:  1,
	}, summary)
}

func TestClassifyOpenBGPDRouteFamily(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		wantAFI  string
		wantSAFI string
		ok       bool
	}{
		{name: "ipv4 unicast", prefix: "203.0.113.0/24", wantAFI: "ipv4", wantSAFI: "unicast", ok: true},
		{name: "ipv6 unicast", prefix: "2001:db8::/64", wantAFI: "ipv6", wantSAFI: "unicast", ok: true},
		{name: "ipv4 vpn", prefix: "rd 65010:1 198.51.100.0/24", wantAFI: "ipv4", wantSAFI: "vpn", ok: true},
		{name: "ipv6 vpn", prefix: "rd 65010:1 2001:db8::/64", wantAFI: "ipv6", wantSAFI: "vpn", ok: true},
		{name: "evpn type2", prefix: "[2]:[rd 65010:1]:[]:[100]:[48]:[02:00:00:00:00:01]:[32]:[192.0.2.10]", wantAFI: "l2vpn", wantSAFI: "evpn", ok: true},
		{name: "evpn type3", prefix: "[3]:[rd 65010:1]:[32]:[192.0.2.11]", wantAFI: "l2vpn", wantSAFI: "evpn", ok: true},
		{name: "empty", prefix: "", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			afi, safi, ok := classifyOpenBGPDRouteFamily(tt.prefix)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.wantAFI, afi)
			assert.Equal(t, tt.wantSAFI, safi)
		})
	}
}

func TestOpenBGPDRIBCacheRequiresMatchingRequestedFamilies(t *testing.T) {
	var cache openbgpdRIBCache
	requested := []string{"default_ipv4_unicast"}
	cache.set(requested, map[string]openbgpdRIBSummary{
		"default_ipv4_unicast": {RIBRoutes: 1},
	})

	cached, ok := cache.getIfFresh(time.Minute, []string{"default_ipv4_unicast"})
	require.True(t, ok)
	assert.Equal(t, int64(1), cached["default_ipv4_unicast"].RIBRoutes)

	_, ok = cache.getIfFresh(time.Minute, []string{"default_ipv4_unicast", "default_ipv6_unicast"})
	assert.False(t, ok)
}

func TestOpenBGPDRIBFilterForFamilyID(t *testing.T) {
	tests := []struct {
		name     string
		familyID string
		want     string
		ok       bool
	}{
		{name: "ipv4 unicast", familyID: "default_ipv4_unicast", want: "ipv4", ok: true},
		{name: "ipv6 unicast", familyID: "default_ipv6_unicast", want: "ipv6", ok: true},
		{name: "ipv4 vpn", familyID: "default_ipv4_vpn", want: "vpnv4", ok: true},
		{name: "ipv6 vpn", familyID: "default_ipv6_vpn", want: "vpnv6", ok: true},
		{name: "evpn unsupported", familyID: "default_l2vpn_evpn", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := openbgpdRIBFilterForFamilyID(tt.familyID)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}
