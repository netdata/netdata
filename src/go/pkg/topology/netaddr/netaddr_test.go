// SPDX-License-Identifier: GPL-3.0-or-later

package netaddr

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNetworkAddress(t *testing.T) {
	tests := map[string]struct {
		ip     string
		mask   string
		want   string
		wantOK bool
	}{
		"ipv4": {
			ip:     "10.0.0.2",
			mask:   "255.255.255.252",
			want:   "10.0.0.0",
			wantOK: true,
		},
		"ipv4 mapped": {
			ip:     "::ffff:10.0.0.2",
			mask:   "::ffff:255.255.255.252",
			want:   "10.0.0.0",
			wantOK: true,
		},
		"family mismatch": {
			ip:     "10.0.0.2",
			mask:   "ffff:ffff:ffff:ffff::",
			wantOK: false,
		},
		"ipv6": {
			ip:     "2001:db8::2",
			mask:   "ffff:ffff:ffff:ffff::",
			want:   "2001:db8::",
			wantOK: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			network, ok := NetworkAddress(netip.MustParseAddr(tt.ip), netip.MustParseAddr(tt.mask))
			require.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				require.Equal(t, tt.want, network.String())
			}
		})
	}
}

func TestMaskToCIDRPrefix(t *testing.T) {
	tests := map[string]struct {
		mask    string
		want    int
		wantErr bool
	}{
		"ipv4 /30": {
			mask: "255.255.255.252",
			want: 30,
		},
		"ipv4 /0": {
			mask: "0.0.0.0",
			want: 0,
		},
		"ipv6 /64": {
			mask: "ffff:ffff:ffff:ffff::",
			want: 64,
		},
		"non-contiguous": {
			mask:    "255.0.255.0",
			wantErr: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			prefix, err := MaskToCIDRPrefix(netip.MustParseAddr(tt.mask))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, prefix)
		})
	}
}
