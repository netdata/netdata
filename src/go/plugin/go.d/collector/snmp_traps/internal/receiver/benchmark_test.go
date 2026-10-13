// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"fmt"
	"net"
	"testing"

	"github.com/gosnmp/gosnmp"
)

func benchmarkPDUs(count int) []gosnmp.SnmpPDU {
	out := make([]gosnmp.SnmpPDU, count)
	for i := range out {
		out[i] = gosnmp.SnmpPDU{
			Name:  fmt.Sprintf("1.3.6.1.2.1.2.2.1.%d", i+1),
			Type:  gosnmp.Integer,
			Value: int(i),
		}
	}
	return out
}

func BenchmarkDecodeTrap(b *testing.B) {
	cases := map[string]int{
		"Varbinds=2":   0,
		"Varbinds=5":   3,
		"Varbinds=10":  8,
		"Varbinds=20":  18,
		"Varbinds=50":  48,
		"Varbinds=256": 254,
	}
	for name, count := range cases {
		b.Run(name, func(b *testing.B) {
			data := buildV2cTrap(b, "public", "1.3.6.1.6.3.1.1.5.1", benchmarkPDUs(count)...)
			peer := net.ParseIP("10.1.2.3")
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = decodeTrap(data, peer, nil)
			}
			b.StopTimer()
			b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "traps/s")
		})
	}
}

func BenchmarkBERRejection(b *testing.B) {
	cases := map[string][]byte{
		"Oversized":          make([]byte, maxDatagramSize+1),
		"DepthOverLimit":     nestedSequence(maxNestingDepth + 1),
		"OIDTooLong":         berTLV(tagSequence, berTLV(tagOID, make([]byte, maxOIDEncodedLen+1))),
		"OctetStringTooLong": berTLV(tagSequence, berTLV(tagOctetStr, make([]byte, maxOctetStringLen+1))),
		"TrailingData":       append(buildV2cTrap(b, "public", "1.3.6.1.6.3.1.1.5.1"), 0x00),
		"IndefiniteLength":   {tagSequence, 0x80, 0x00, 0x00},
		"Truncated":          {0x30, 0x01, 0x02},
	}

	for name, data := range cases {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = decodePacket(data, nil)
			}
			b.StopTimer()
			b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "rejected/s")
		})
	}
}
