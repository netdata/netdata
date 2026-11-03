// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// UDP datagram framing without the socket, for single-record and batched
// datagrams, handing records to a counting ingest function. The collector's
// socket benchmarks cover the complete path; ns/op is a development-machine
// trend, never a CI timing gate. ns/op and allocations are per record.
func BenchmarkDatagram(b *testing.B) {
	for _, perDatagram := range []int{1, 16} {
		b.Run(fmt.Sprint(perDatagram), func(b *testing.B) {
			var records int
			s := &Server{
				sink:  func(string, time.Time) { records++ },
				stats: &Stats{},
				now:   time.Now,
			}
			var payloads [][]byte
			for i := 0; i < 960; i += perDatagram {
				lines := make([]string, perDatagram)
				for j := range lines {
					lines[j] = fmt.Sprintf("app.metric%d:%d|c", i+j, j)
				}
				payloads = append(payloads, []byte(strings.Join(lines, "\n")))
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i += perDatagram {
				s.datagram(payloads[(i/perDatagram)%len(payloads)])
			}
			b.StopTimer()
			if records < b.N {
				b.Fatalf("framed %d of %d records", records, b.N)
			}
		})
	}
}
