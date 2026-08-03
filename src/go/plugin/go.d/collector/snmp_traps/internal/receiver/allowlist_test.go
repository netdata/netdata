// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"net"
	"net/netip"
	"testing"
)

func TestAllowlistCIDR(t *testing.T) {
	al := NewAllowlist([]netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("192.168.1.0/24"),
	}, nil)

	tests := map[string]struct {
		addr string
		want bool
	}{
		"inside 10.x":    {addr: "10.1.2.3", want: true},
		"inside 192.168": {addr: "192.168.1.42", want: true},
		"outside":        {addr: "172.16.0.1", want: false},
		"loopback":       {addr: "127.0.0.1", want: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			addr := netip.MustParseAddr(tc.addr)
			if got := al.AllowedSource(addr); got != tc.want {
				t.Errorf("AllowedSource(%s) = %v, want %v", tc.addr, got, tc.want)
			}
		})
	}
}

func TestAllowlistDefaultIncludesIPv6(t *testing.T) {
	prefixes, err := NormalizeSourceAllowlist(nil)
	if err != nil {
		t.Fatalf("NormalizeSourceAllowlist failed: %v", err)
	}
	al := NewAllowlist(prefixes, nil)

	if !al.AllowedSource(netip.MustParseAddr("2001:db8::1")) {
		t.Fatal("default allowlist should accept IPv6")
	}
	if !al.AllowedSource(netip.MustParseAddr("192.0.2.1")) {
		t.Fatal("default allowlist should accept IPv4")
	}
}

func TestPacketSourceAddrFallsBackToPeerIP(t *testing.T) {
	addr, ok := packetSourceAddr(net.ParseIP("::ffff:10.1.2.3"), nil)
	if !ok {
		t.Fatal("expected source address from peerIP")
	}
	if got := addr.String(); got != "10.1.2.3" {
		t.Fatalf("source addr = %q, want 10.1.2.3", got)
	}
}

func TestAllowlistCommunity(t *testing.T) {
	al := NewAllowlist(nil, []string{"allowed-a", "allowed-b"})

	tests := map[string]struct {
		value string
		want  bool
	}{
		"allowed-a": {value: "allowed-a", want: true},
		"allowed-b": {value: "allowed-b", want: true},
		"denied":    {value: "denied", want: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := al.AllowedCommunity(tc.value); got != tc.want {
				t.Errorf("AllowedCommunity(%s) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}
