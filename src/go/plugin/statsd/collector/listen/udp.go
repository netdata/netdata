// SPDX-License-Identifier: GPL-3.0-or-later

package listen

import (
	"net"
	"strings"
)

// readUDP reads datagrams until the listener closes or fails permanently.
func (s *server) readUDP(conn net.PacketConn, listener string) {
	buf := make([]byte, s.maxRecord+1)
	for {
		n, _, err := conn.ReadFrom(buf)
		s.stats.udpBytes.Add(uint64(n))
		if n > s.maxRecord {
			// A datagram exceeds the record bound only when the read truncated it.
			// Reject it whole instead of admitting a prefix; a truncating read may
			// also report an error, but the socket remains usable.
			s.receiver.reject(rejectOversize)
			continue
		}
		if n > 0 {
			s.datagram(buf[:n])
		}
		if err != nil && !s.recoverable(err, listener) {
			return
		}
	}
}

// datagram frames one UDP payload. LF or CRLF ends a record, and the datagram
// boundary also ends its final record. Datagrams are never joined. The payload is
// copied once, so records are substrings of a string the receive buffer does not own.
func (s *server) datagram(payload []byte) {
	now := s.now()
	for text := string(payload); len(text) > 0; {
		line, rest, terminated := strings.Cut(text, "\n")
		if terminated {
			line = strings.TrimSuffix(line, "\r")
		}
		s.ingest(line, now)
		text = rest
	}
}
