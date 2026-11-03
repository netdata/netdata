// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUDPFraming(t *testing.T) {
	f := start(t, func(cfg *Config) { cfg.MaxRecord = 64 })
	// LF, CRLF, and the datagram boundary each end a record; empty lines are not records.
	f.sendUDP(t, "a:1|c\nb:2|c\r\n\nc:3|c")
	want := framing{
		records: []string{"a:1|c", "b:2|c", "c:3|c"},
	}
	f.waitFraming(t, want)
	// A bare CR is record content, not a terminator.
	f.sendUDP(t, "bad record\nd:4|c\ne:5|c\r")
	want.records = append(want.records, "bad record", "d:4|c", "e:5|c\r")
	f.waitFraming(t, want)
	// A datagram beyond the record bound was truncated by the read: no prefix is handed over.
	f.sendUDP(t, strings.Repeat("f:1|c\n", 12))
	want.oversize = 1
	f.waitFraming(t, want)
	f.sendUDP(t, "g:1|c")
	want.records = append(want.records, "g:1|c")
	f.waitFraming(t, want)
	// A datagram of exactly the bound is complete.
	exact := strings.Repeat("h", 64-len(":1|c")) + ":1|c"
	f.sendUDP(t, exact)
	want.records = append(want.records, exact)
	f.waitFraming(t, want)
}

func TestTCPFraming(t *testing.T) {
	const bound = 32
	f := start(t, func(cfg *Config) { cfg.MaxRecord = bound })
	exact := strings.Repeat("x", bound-len(":1|c")) + ":1|c"
	require.Len(t, exact, bound)
	over := strings.Repeat("y", bound+1-len(":1|c")) + ":1|c"

	conn := f.dialTCP(t)
	for _, chunk := range []string{
		"a:1|c\nb:2", "|c\r\n", // A record split across reads.
		"bad record\nc:3|c\n",       // Malformed neighbor: framing hands it over.
		exact + "\r\n", over + "\n", // Exact bound accepted; one byte over rejected.
		strings.Repeat("z", 3*bound), "z\nd:4|c\n", // Oversize discarded through its newline.
		"e:5|c", // Unterminated at EOF.
	} {
		_, err := conn.Write([]byte(chunk))
		require.NoError(t, err)
		time.Sleep(time.Millisecond)
	}
	require.NoError(t, conn.(*net.TCPConn).CloseWrite())
	want := framing{
		records:      []string{"a:1|c", "b:2|c", "bad record", "c:3|c", exact, "d:4|c"},
		oversize:     2,
		unterminated: 1,
	}
	f.waitFraming(t, want)

	// EOF while discarding an oversize record counts it once, not also as unterminated.
	discarding := f.dialTCP(t)
	_, err := discarding.Write([]byte(strings.Repeat("w", 3*bound)))
	require.NoError(t, err)
	require.NoError(t, discarding.(*net.TCPConn).CloseWrite())
	want.oversize = 3
	f.waitFraming(t, want)
}

func TestTCPConnectionCapKeepsQuietClients(t *testing.T) {
	f := start(t, func(cfg *Config) { cfg.MaxConns = 2 })
	first, second := f.dialTCP(t), f.dialTCP(t)
	require.Eventually(t, func() bool { return f.stats.TCPConnections.Load() == 2 }, waitFor, time.Millisecond)
	time.Sleep(50 * time.Millisecond) // Quiet clients are not timed out.

	refused := f.dialTCP(t)
	require.NoError(t, refused.SetReadDeadline(time.Now().Add(waitFor)))
	_, err := refused.Read(make([]byte, 1))
	require.Error(t, err, "a client beyond the cap is closed")
	require.Eventually(t, func() bool { return f.stats.TCPRefused.Load() == 1 }, waitFor, time.Millisecond)

	var want framing
	for i, conn := range []net.Conn{first, second} {
		_, err := fmt.Fprintf(conn, "quiet%d:1|c\n", i)
		require.NoError(t, err)
		want.records = append(want.records, fmt.Sprintf("quiet%d:1|c", i))
		f.waitFraming(t, want)
	}

	// A disconnect is routine: it frees a slot and the server keeps serving.
	require.NoError(t, first.Close())
	require.Eventually(t, func() bool { return f.stats.TCPConnections.Load() == 1 }, waitFor, time.Millisecond)
	next := f.dialTCP(t)
	_, err = next.Write([]byte("next:1|c\n"))
	require.NoError(t, err)
	want.records = append(want.records, "next:1|c")
	f.waitFraming(t, want)
	require.Equal(t, int64(2), f.stats.TCPConnections.Load())
	require.Equal(t, uint64(1), f.stats.TCPRefused.Load())
	select {
	case <-f.s.Failed():
		t.Fatalf("client disconnect failed the server: %v", f.s.Err())
	default:
	}
}
