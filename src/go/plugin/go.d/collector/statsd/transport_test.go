// SPDX-License-Identifier: GPL-3.0-or-later

package statsd

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUDPFraming(t *testing.T) {
	f := startRuntime(t, func(c *Collector) { c.maxRecord = 64 })
	// LF, CRLF, and the datagram boundary each end a record; empty lines are not records.
	f.sendUDP(t, "a:1|c\nb:2|c\r\n\nc:3|c")
	f.waitCounts(t, receiverCounts{
		accepted: 3,
	})
	// Malformed neighbors reject independently. A bare CR is record content, not a terminator.
	f.sendUDP(t, "bad record\nd:4|c\ne:5|c\r")
	f.waitCounts(t, receiverCounts{
		accepted: 4,
		rejected: map[rejection]uint64{rejectSyntax: 2},
	})
	// A datagram beyond the record bound was truncated by the read: no prefix is admitted.
	f.sendUDP(t, strings.Repeat("f:1|c\n", 12))
	f.waitCounts(t, receiverCounts{
		accepted: 4,
		rejected: map[rejection]uint64{rejectSyntax: 2, rejectOversize: 1},
	})
	f.sendUDP(t, "g:1|c")
	f.waitCounts(t, receiverCounts{
		accepted: 5,
		rejected: map[rejection]uint64{rejectSyntax: 2, rejectOversize: 1},
	})
	// A datagram of exactly the bound is complete.
	f.sendUDP(t, strings.Repeat("h", 64-len(":1|c"))+":1|c")
	f.waitCounts(t, receiverCounts{
		accepted: 6,
		rejected: map[rejection]uint64{rejectSyntax: 2, rejectOversize: 1},
	})
	f.collect(t, false, false)
	for name, want := range map[string]float64{"a": 1, "b": 2, "c": 3, "d": 4, "g": 1} {
		value(t, f.c, "c.total."+name, want, nil)
	}
	_, ok := f.c.store.Read().Value("c.total.f", nil)
	assert.False(t, ok)
}

func TestTCPFraming(t *testing.T) {
	const bound = 32
	f := startRuntime(t, func(c *Collector) { c.maxRecord = bound })
	exact := strings.Repeat("x", bound-len(":1|c")) + ":1|c"
	require.Len(t, exact, bound)
	over := strings.Repeat("y", bound+1-len(":1|c")) + ":1|c"

	conn := f.dialTCP(t)
	for _, chunk := range []string{
		"a:1|c\nb:2", "|c\r\n", // A record split across reads.
		"bad record\nc:3|c\n",       // Malformed neighbor.
		exact + "\r\n", over + "\n", // Exact bound accepted; one byte over rejected.
		strings.Repeat("z", 3*bound), "z\nd:4|c\n", // Oversize discarded through its newline.
		"e:5|c", // Unterminated at EOF.
	} {
		_, err := conn.Write([]byte(chunk))
		require.NoError(t, err)
		time.Sleep(time.Millisecond)
	}
	require.NoError(t, conn.(*net.TCPConn).CloseWrite())
	f.waitCounts(t, receiverCounts{
		accepted: 5,
		rejected: map[rejection]uint64{
			rejectSyntax:       1,
			rejectOversize:     2,
			rejectUnterminated: 1,
		},
	})
	f.collect(t, false, false)
	for name, want := range map[string]float64{"a": 1, "b": 2, "c": 3, "d": 4} {
		value(t, f.c, "c.total."+name, want, nil)
	}
	value(t, f.c, "c.total."+exact[:len(exact)-len(":1|c")], 1, nil)
	_, ok := f.c.store.Read().Value("c.total.e", nil)
	assert.False(t, ok, "a fragment without newline at EOF is not a record")

	// EOF while discarding an oversize record counts it once, not also as unterminated.
	discarding := f.dialTCP(t)
	_, err := discarding.Write([]byte(strings.Repeat("w", 3*bound)))
	require.NoError(t, err)
	require.NoError(t, discarding.(*net.TCPConn).CloseWrite())
	f.waitCounts(t, receiverCounts{
		accepted: 5,
		rejected: map[rejection]uint64{
			rejectSyntax:       1,
			rejectOversize:     3,
			rejectUnterminated: 1,
		},
	})
}

func TestTCPConnectionCapKeepsQuietClients(t *testing.T) {
	f := startRuntime(t, func(c *Collector) { c.MaxTCPConnections = 2 })
	first, second := f.dialTCP(t), f.dialTCP(t)
	require.Eventually(t, func() bool { return f.c.diagnostics.tcpConnections.Load() == 2 }, waitFor, time.Millisecond)
	time.Sleep(50 * time.Millisecond) // Quiet clients are not timed out.

	refused := f.dialTCP(t)
	require.NoError(t, refused.SetReadDeadline(time.Now().Add(waitFor)))
	_, err := refused.Read(make([]byte, 1))
	require.Error(t, err, "a client beyond the cap is closed")
	require.Eventually(t, func() bool { return f.c.diagnostics.tcpRefused.Load() == 1 }, waitFor, time.Millisecond)

	for i, conn := range []net.Conn{first, second} {
		_, err := fmt.Fprintf(conn, "quiet%d:1|c\n", i)
		require.NoError(t, err)
	}
	f.waitCounts(t, receiverCounts{
		accepted: 2,
	})

	// A disconnect is routine: it frees a slot and the receiver keeps serving.
	require.NoError(t, first.Close())
	require.Eventually(t, func() bool { return f.c.diagnostics.tcpConnections.Load() == 1 }, waitFor, time.Millisecond)
	next := f.dialTCP(t)
	_, err = next.Write([]byte("next:1|c\n"))
	require.NoError(t, err)
	f.waitCounts(t, receiverCounts{
		accepted: 3,
	})
	f.collect(t, false, false)
	value(t, f.c, "c.total.next", 1, nil)
	value(t, f.c, "receiver.tcp_connections", 2, nil)
	value(t, f.c, "receiver.tcp_connections_limit", 2, nil)
	value(t, f.c, "receiver.tcp_connections_refused", 1, nil)
	select {
	case err := <-f.done:
		t.Fatalf("client disconnect ended Run: %v", err)
	default:
	}
}
