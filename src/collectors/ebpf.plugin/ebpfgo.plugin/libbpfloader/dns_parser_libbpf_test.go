//go:build netdata_ebpf_libbpf

package libbpfloader

import (
	"testing"
	"unsafe"
)

// validDNSQuery is a well-formed Ethernet/IPv4/UDP packet carrying a DNS
// standard query for "foo." A IN.
//
//	14 (Ethernet) + 20 (IPv4) + 8 (UDP) + 12 (DNS header) + 9 (question) = 63 bytes
var validDNSQuery = []byte{
	// Ethernet (14 bytes)
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // dst MAC
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // src MAC
	0x08, 0x00, // EtherType IPv4

	// IPv4 (20 bytes) — checksum unchecked by parser
	0x45, 0x00, 0x00, 0x31, // version+IHL=5, DSCP, total-length=49
	0x00, 0x00, 0x40, 0x00, // ID, DF flag, fragment offset 0
	0x40, 0x11, 0x00, 0x00, // TTL=64, protocol=UDP(17), checksum=0
	0xC0, 0xA8, 0x01, 0x01, // src 192.168.1.1
	0x08, 0x08, 0x08, 0x08, // dst 8.8.8.8

	// UDP (8 bytes)
	0x04, 0xD2, // src port 1234
	0x00, 0x35, // dst port 53
	0x00, 0x1D, // length 29
	0x00, 0x00, // checksum 0

	// DNS header (12 bytes)
	0xAB, 0xCD, // TX ID
	0x01, 0x00, // flags: standard query (QR=0)
	0x00, 0x01, // QDCOUNT=1
	0x00, 0x00, // ANCOUNT=0
	0x00, 0x00, // NSCOUNT=0
	0x00, 0x00, // ARCOUNT=0

	// DNS question: "foo." A IN (9 bytes)
	0x03, 0x66, 0x6F, 0x6F, 0x00, // \x03foo\x00
	0x00, 0x01, // type A
	0x00, 0x01, // class IN
}

func mutated(src []byte, off int, b ...byte) []byte {
	dst := append([]byte{}, src...)
	copy(dst[off:], b)
	return dst
}

// TestDNSParseRawPacketBoundaries verifies that dns_parse_raw_packet rejects
// malformed or undersized packets without crashing, and returns the expected
// bool result for each case.
func TestDNSParseRawPacketBoundaries(t *testing.T) {
	rt := dnsAllocTestRuntime()
	if rt == unsafe.Pointer(nil) {
		t.Fatal("alloc test runtime returned nil")
	}
	defer dnsFreeTestRuntime(rt)

	tests := map[string]struct {
		pkt  []byte
		want bool
	}{
		"empty packet": {
			pkt: []byte{}, want: false,
		},
		"13 bytes — one below Ethernet minimum": {
			pkt: validDNSQuery[:13], want: false,
		},
		"14 bytes — Ethernet only, no IP payload": {
			pkt: validDNSQuery[:14], want: false,
		},
		"truncated IPv4 header (28 bytes)": {
			// 14 Eth + 14 bytes of IP header — IHL=5 needs 20
			pkt: validDNSQuery[:28], want: false,
		},
		"IPv4 IHL larger than packet": {
			// IHL=15 (60 bytes) in a 34-byte packet
			pkt: func() []byte {
				p := make([]byte, 34)
				copy(p, validDNSQuery[:34])
				p[14] = 0x4F // version=4, IHL=15
				return p
			}(),
			want: false,
		},
		"IPv4 version field is 5 (not 4)": {
			pkt:  mutated(validDNSQuery, 14, 0x55), // upper nibble = 5
			want: false,
		},
		"non-DNS port (UDP dst 8080)": {
			// offset 36 = 14(Eth) + 20(IP) + 2 (past src port)
			pkt:  mutated(validDNSQuery, 36, 0x1F, 0x90),
			want: false,
		},
		"IPv6 EtherType but packet too short for IPv6 header": {
			pkt:  mutated(validDNSQuery[:14], 12, 0x86, 0xDD),
			want: false,
		},
		"VLAN tag (0x8100) without inner payload": {
			pkt: []byte{
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // dst MAC
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // src MAC
				0x81, 0x00, // EtherType: 802.1Q VLAN
				0x00, 0x01, // VLAN TCI
				0x08, 0x00, // inner EtherType: IPv4 — no payload
			},
			want: false,
		},
		"truncated DNS payload still counted as DNS transport": {
			// Valid Ethernet+IP+UDP headers, port 53, only 8 DNS bytes (need 12)
			// dns_parse_payload returns false; dns_parse_raw_packet returns true.
			pkt:  validDNSQuery[:14+20+8+8],
			want: true,
		},
		"valid full DNS query": {
			pkt: validDNSQuery, want: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := dnsTestParseRawPacket(rt, tc.pkt)
			if got != tc.want {
				t.Errorf("parse_raw_packet(%d bytes) = %v, want %v", len(tc.pkt), got, tc.want)
			}
		})
	}
}

// TestDNSReadNameBoundaries exercises dns_read_name against truncated,
// looping, and oversized-label inputs to confirm it does not crash and
// returns 0 (error) for every malformed case.
func TestDNSReadNameBoundaries(t *testing.T) {
	call := func(msg []byte, offset, outSize int) int {
		return dnsTestReadName(msg, offset, outSize)
	}

	t.Run("empty message", func(t *testing.T) {
		if got := call(nil, 0, 256); got != 0 {
			t.Errorf("got %d, want 0", got)
		}
	})

	t.Run("offset at end of message", func(t *testing.T) {
		if got := call([]byte{0x00}, 1, 256); got != 0 {
			t.Errorf("got %d, want 0", got)
		}
	})

	t.Run("compression pointer — second byte missing", func(t *testing.T) {
		if got := call([]byte{0xC0}, 0, 256); got != 0 {
			t.Errorf("got %d, want 0", got)
		}
	})

	t.Run("self-referential compression pointer loop", func(t *testing.T) {
		// 0xC0 0x00 at offset 0 points back to offset 0 — hits 32-jump limit
		if got := call([]byte{0xC0, 0x00}, 0, 256); got != 0 {
			t.Errorf("got %d, want 0", got)
		}
	})

	t.Run("label length exceeds remaining message bytes", func(t *testing.T) {
		if got := call([]byte{0x10, 'a', 'b'}, 0, 256); got != 0 {
			t.Errorf("got %d, want 0", got)
		}
	})

	t.Run("label length > 63 (RFC max)", func(t *testing.T) {
		if got := call([]byte{0x40, 0x00}, 0, 256); got != 0 {
			t.Errorf("got %d, want 0", got)
		}
	})

	t.Run("out buffer too small for label", func(t *testing.T) {
		// Label "foob" (4 chars) needs 5 bytes in out (4+NUL); max_out=4 → overflow
		if got := call([]byte{0x04, 'f', 'o', 'o', 'b', 0x00}, 0, 4); got != 0 {
			t.Errorf("got %d, want 0", got)
		}
	})

	t.Run("root label only — empty name", func(t *testing.T) {
		if got := call([]byte{0x00}, 0, 256); got != 1 {
			t.Errorf("got %d, want 1", got)
		}
	})

	t.Run("valid single-label foo", func(t *testing.T) {
		// \x03foo\x00 → 5 bytes consumed
		if got := call([]byte{0x03, 'f', 'o', 'o', 0x00}, 0, 256); got != 5 {
			t.Errorf("got %d, want 5", got)
		}
	})

	t.Run("valid two-label name go.dev", func(t *testing.T) {
		// \x02go\x03dev\x00 → 8 bytes consumed
		msg := []byte{0x02, 'g', 'o', 0x03, 'd', 'e', 'v', 0x00}
		if got := call(msg, 0, 256); got != 8 {
			t.Errorf("got %d, want 8", got)
		}
	})
}
