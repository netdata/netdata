// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopologyCache_ModernIPAddressEligibilityAndPrefix(t *testing.T) {
	tests := map[string]struct {
		mutate      func(map[string]string)
		wantAddress bool
		wantNetmask string
	}{
		"preferred unicast active": {
			wantAddress: true,
			wantNetmask: "255.255.255.240",
		},
		"deprecated remains usable": {
			mutate:      func(tags map[string]string) { tags[tagTopoIPStatus] = "deprecated" },
			wantAddress: true,
			wantNetmask: "255.255.255.240",
		},
		"zero pointer keeps inventory": {
			mutate:      func(tags map[string]string) { tags[tagTopoIPPrefix] = "0.0" },
			wantAddress: true,
		},
		"missing pointer keeps inventory": {
			mutate:      func(tags map[string]string) { delete(tags, tagTopoIPPrefix) },
			wantAddress: true,
		},
		"wrong pointer target keeps inventory": {
			mutate: func(tags map[string]string) {
				tags[tagTopoIPPrefix] = "1.3.6.1.2.1.4.32.1.4.7.1.4.192.0.2.16.28"
			},
			wantAddress: true,
		},
		"pointer interface mismatch keeps inventory": {
			mutate: func(tags map[string]string) {
				tags[tagTopoIPPrefix] = "1.3.6.1.2.1.4.32.1.5.8.1.4.192.0.2.16.28"
			},
			wantAddress: true,
		},
		"pointer address type mismatch keeps inventory": {
			mutate: func(tags map[string]string) {
				tags[tagTopoIPPrefix] = "1.3.6.1.2.1.4.32.1.5.7.2.4.192.0.2.16.28"
			},
			wantAddress: true,
		},
		"pointer address length mismatch keeps inventory": {
			mutate: func(tags map[string]string) {
				tags[tagTopoIPPrefix] = "1.3.6.1.2.1.4.32.1.5.7.1.16.192.0.2.16.28"
			},
			wantAddress: true,
		},
		"pointer with truncated index keeps inventory": {
			mutate: func(tags map[string]string) {
				tags[tagTopoIPPrefix] = "1.3.6.1.2.1.4.32.1.5.7.1.4.192.0.2.28"
			},
			wantAddress: true,
		},
		"pointer with trailing index components keeps inventory": {
			mutate: func(tags map[string]string) {
				tags[tagTopoIPPrefix] = "1.3.6.1.2.1.4.32.1.5.7.1.4.192.0.2.16.28.99"
			},
			wantAddress: true,
		},
		"pointer with invalid octet keeps inventory": {
			mutate: func(tags map[string]string) {
				tags[tagTopoIPPrefix] = "1.3.6.1.2.1.4.32.1.5.7.1.4.192.0.256.16.28"
			},
			wantAddress: true,
		},
		"non-network prefix keeps inventory": {
			mutate: func(tags map[string]string) {
				tags[tagTopoIPPrefix] = "1.3.6.1.2.1.4.32.1.5.7.1.4.192.0.2.17.28"
			},
			wantAddress: true,
		},
		"prefix outside address keeps inventory": {
			mutate: func(tags map[string]string) {
				tags[tagTopoIPPrefix] = "1.3.6.1.2.1.4.32.1.5.7.1.4.198.51.100.0.24"
			},
			wantAddress: true,
		},
		"invalid prefix length keeps inventory": {
			mutate: func(tags map[string]string) {
				tags[tagTopoIPPrefix] = "1.3.6.1.2.1.4.32.1.5.7.1.4.192.0.2.16.33"
			},
			wantAddress: true,
		},
		"anycast is not a device-unique identity": {
			mutate: func(tags map[string]string) { tags[tagTopoIPType] = "anycast" },
		},
		"broadcast is not a device-unique identity": {
			mutate: func(tags map[string]string) { tags[tagTopoIPType] = "broadcast" },
		},
		"invalid address is unusable": {
			mutate: func(tags map[string]string) { tags[tagTopoIPStatus] = "invalid" },
		},
		"optimistic address is not yet usable": {
			mutate: func(tags map[string]string) { tags[tagTopoIPStatus] = "optimistic" },
		},
		"inactive conceptual row is unusable": {
			mutate: func(tags map[string]string) { tags[tagTopoIPRow] = "notInService" },
		},
		"missing type fails closed": {
			mutate: func(tags map[string]string) { delete(tags, tagTopoIPType) },
		},
		"missing status fails closed": {
			mutate: func(tags map[string]string) { delete(tags, tagTopoIPStatus) },
		},
		"missing row status fails closed": {
			mutate: func(tags map[string]string) { delete(tags, tagTopoIPRow) },
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			cache := newTopologyBuilder()
			tags := modernIPv4Tags("192.0.2.17", "7", "1.3.6.1.2.1.4.32.1.5.7.1.4.192.0.2.16.28")
			if test.mutate != nil {
				test.mutate(tags)
			}

			cache.updateIfIndexByIP(tags)
			cache.finalize()

			if !test.wantAddress {
				assert.NotContains(t, cache.ipAddressesByIP, "192.0.2.17")
				return
			}
			assert.Equal(t, "7", cache.ipIfIndex("192.0.2.17"))
			assert.Equal(t, test.wantNetmask, cache.ipNetmask("192.0.2.17"))
			_, hasL3 := cache.ipL3Interface("192.0.2.17")
			if test.wantNetmask == "" {
				assert.False(t, hasL3)
			} else {
				assert.True(t, hasL3)
			}
		})
	}
}

func TestTopologyCache_ModernIPAddressRejectsIPv6(t *testing.T) {
	cache := newTopologyBuilder()
	cache.updateIfIndexByIP(modernIPv4Tags(
		"2001:db8::17",
		"7",
		"1.3.6.1.2.1.4.32.1.5.7.2.16.32.1.13.184.0.0.0.0.0.0.0.0.0.0.0.16.64",
	))
	cache.finalize()

	assert.Empty(t, cache.ipAddressesByIP)
}

func TestTopologyCache_ModernIPAddressRejectsMalformedIndexAddress(t *testing.T) {
	for name, address := range map[string]string{
		"extra components": "1.2.3.4.5.6.7.8",
		"invalid octet":    "192.0.2.256",
		"truncated":        "123.45.67",
	} {
		t.Run(name, func(t *testing.T) {
			cache := newTopologyBuilder()
			cache.updateIfIndexByIP(modernIPv4Tags(address, "7", "0.0"))
			cache.finalize()

			assert.Empty(t, cache.ipAddressesByIP)
			assert.Empty(t, cache.localDevice.ManagementAddresses)
			assert.Empty(t, cache.trapMatchMethodByIP)
		})
	}
}

func TestTopologyCache_IPAddressMergeIsOrderIndependent(t *testing.T) {
	tests := map[string]struct {
		legacy      map[string]string
		modern      map[string]string
		wantIfIndex string
		wantMask    string
	}{
		"legacy mask wins": {
			legacy:      legacyIPv4Tags("192.0.2.17", "7", "255.255.255.0"),
			modern:      modernIPv4Tags("192.0.2.17", "7", "1.3.6.1.2.1.4.32.1.5.7.1.4.192.0.2.16.28"),
			wantIfIndex: "7",
			wantMask:    "255.255.255.0",
		},
		"modern fills missing legacy mask": {
			legacy:      legacyIPv4Tags("192.0.2.17", "7", ""),
			modern:      modernIPv4Tags("192.0.2.17", "7", "1.3.6.1.2.1.4.32.1.5.7.1.4.192.0.2.16.28"),
			wantIfIndex: "7",
			wantMask:    "255.255.255.240",
		},
		"modern fills invalid legacy mask": {
			legacy:      legacyIPv4Tags("192.0.2.17", "7", "255.0.255.0"),
			modern:      modernIPv4Tags("192.0.2.17", "7", "1.3.6.1.2.1.4.32.1.5.7.1.4.192.0.2.16.28"),
			wantIfIndex: "7",
			wantMask:    "255.255.255.240",
		},
		"conflicting modern interface cannot supply mask": {
			legacy:      legacyIPv4Tags("192.0.2.17", "7", ""),
			modern:      modernIPv4Tags("192.0.2.17", "8", "1.3.6.1.2.1.4.32.1.5.8.1.4.192.0.2.16.28"),
			wantIfIndex: "7",
		},
	}

	for name, test := range tests {
		for _, modernFirst := range []bool{false, true} {
			order := "legacy first"
			if modernFirst {
				order = "modern first"
			}
			t.Run(name+"/"+order, func(t *testing.T) {
				cache := newTopologyBuilder()
				if modernFirst {
					cache.updateIfIndexByIP(test.modern)
					cache.updateIfIndexByIP(test.legacy)
				} else {
					cache.updateIfIndexByIP(test.legacy)
					cache.updateIfIndexByIP(test.modern)
				}
				cache.finalize()

				assert.Equal(t, test.wantIfIndex, cache.ipIfIndex("192.0.2.17"))
				assert.Equal(t, test.wantMask, cache.ipNetmask("192.0.2.17"))
				require.Len(t, cache.localDevice.ManagementAddresses, 1)
			})
		}
	}
}

func TestTopologyCache_IPAddressUnknownSourceCannotOverride(t *testing.T) {
	cache := newTopologyBuilder()
	cache.updateIfIndexByIP(legacyIPv4Tags("192.0.2.17", "7", "255.255.255.0"))
	cache.updateIfIndexByIP(map[string]string{
		tagTopoIPAddr:  "192.0.2.17",
		tagTopoIfIndex: "99",
		tagTopoIPMask:  "255.255.0.0",
	})
	cache.finalize()

	assert.Equal(t, "7", cache.ipIfIndex("192.0.2.17"))
	assert.Equal(t, "255.255.255.0", cache.ipNetmask("192.0.2.17"))
}

func TestTopologyCache_IPAddressSameSourceCandidateResolution(t *testing.T) {
	tests := map[string]struct {
		rows        []map[string]string
		wantIfIndex string
		wantMask    string
	}{
		"exact duplicate collapses": {
			rows: []map[string]string{
				legacyIPv4Tags("192.0.2.17", "7", "255.255.255.0"),
				legacyIPv4Tags("192.0.2.17", "7", "255.255.255.0"),
			},
			wantIfIndex: "7",
			wantMask:    "255.255.255.0",
		},
		"conflicting interface indexes fail closed": {
			rows: []map[string]string{
				modernIPv4Tags("192.0.2.17", "7", "1.3.6.1.2.1.4.32.1.5.7.1.4.192.0.2.16.28"),
				modernIPv4Tags("192.0.2.17", "8", "1.3.6.1.2.1.4.32.1.5.8.1.4.192.0.2.16.28"),
			},
		},
		"conflicting masks retain interface inventory": {
			rows: []map[string]string{
				legacyIPv4Tags("192.0.2.17", "7", "255.255.255.0"),
				legacyIPv4Tags("192.0.2.17", "7", "255.255.255.128"),
			},
			wantIfIndex: "7",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			cache := newTopologyBuilder()
			for _, row := range test.rows {
				cache.updateIfIndexByIP(row)
			}
			cache.finalize()

			assert.Equal(t, test.wantIfIndex, cache.ipIfIndex("192.0.2.17"))
			assert.Equal(t, test.wantMask, cache.ipNetmask("192.0.2.17"))
			if test.wantMask == "" {
				_, hasL3 := cache.ipL3Interface("192.0.2.17")
				assert.False(t, hasL3)
			}
		})
	}
}

func TestTopologyCache_LegacyAmbiguityDoesNotFallBackToModern(t *testing.T) {
	tests := map[string]struct {
		legacyRows  []map[string]string
		wantIfIndex string
	}{
		"conflicting legacy interfaces suppress the address": {
			legacyRows: []map[string]string{
				legacyIPv4Tags("192.0.2.17", "7", "255.255.255.0"),
				legacyIPv4Tags("192.0.2.17", "8", "255.255.255.0"),
			},
		},
		"conflicting legacy masks retain inventory without modern prefix": {
			legacyRows: []map[string]string{
				legacyIPv4Tags("192.0.2.17", "7", "255.255.255.0"),
				legacyIPv4Tags("192.0.2.17", "7", "255.255.255.128"),
			},
			wantIfIndex: "7",
		},
	}

	for name, test := range tests {
		for _, modernFirst := range []bool{false, true} {
			order := "legacy_first"
			if modernFirst {
				order = "modern_first"
			}
			t.Run(name+"/"+order, func(t *testing.T) {
				cache := newTopologyBuilder()
				modern := modernIPv4Tags(
					"192.0.2.17",
					"7",
					"1.3.6.1.2.1.4.32.1.5.7.1.4.192.0.2.16.28",
				)
				if modernFirst {
					cache.updateIfIndexByIP(modern)
				}
				for _, row := range test.legacyRows {
					cache.updateIfIndexByIP(row)
				}
				if !modernFirst {
					cache.updateIfIndexByIP(modern)
				}
				cache.finalize()

				assert.Equal(t, test.wantIfIndex, cache.ipIfIndex("192.0.2.17"))
				assert.Empty(t, cache.ipNetmask("192.0.2.17"))
				_, hasL3 := cache.ipL3Interface("192.0.2.17")
				assert.False(t, hasL3)
			})
		}
	}
}

func modernIPv4Tags(ip, ifIndex, prefixPointer string) map[string]string {
	return map[string]string{
		tagTopoIPAddr:   ip,
		tagTopoIfIndex:  ifIndex,
		tagTopoIPSource: topoIPSourceModern,
		tagTopoIPType:   "unicast",
		tagTopoIPPrefix: prefixPointer,
		tagTopoIPStatus: "preferred",
		tagTopoIPRow:    "active",
	}
}

func legacyIPv4Tags(ip, ifIndex, mask string) map[string]string {
	return map[string]string{
		tagTopoIPAddr:   ip,
		tagTopoIfIndex:  ifIndex,
		tagTopoIPMask:   mask,
		tagTopoIPSource: topoIPSourceLegacy,
	}
}
