// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"io"
	"net"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const waitFor = 5 * time.Second

// recorder collects ingested records in arrival order.
type recorder struct {
	mu      sync.Mutex
	records []string
}

func (r *recorder) ingest(record string, _ time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, record)
}

func (r *recorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.records)
}

// fixture is a Server bound to one UDP and one TCP endpoint on a loopback port.
type fixture struct {
	s         *Server
	stats     *Stats
	rec       *recorder
	addr      string
	closeOnce sync.Once
}

// bind acquires the endpoints without starting readers, so a test may replace
// sockets first. configure may adjust the configuration before Listen.
func bind(t testing.TB, configure func(*Config)) *fixture {
	t.Helper()
	f := &fixture{
		stats: &Stats{},
		rec:   &recorder{},
		addr:  freeAddress(t),
	}
	cfg := Config{
		Endpoints: []Endpoint{
			{Network: "udp", Address: f.addr, Name: "udp endpoint"},
			{Network: "tcp", Address: f.addr, Name: "tcp endpoint"},
		},
		MaxRecord: MaxRecordSize,
		MaxConns:  64,
		Stats:     f.stats,
		Ingest:    f.rec.ingest,
		Now:       time.Now,
	}
	if configure != nil {
		configure(&cfg)
	}
	s, err := Listen(t.Context(), cfg)
	require.NoError(t, err)
	f.s = s
	t.Cleanup(f.close)
	return f
}

// start binds and starts the readers.
func start(t testing.TB, configure func(*Config)) *fixture {
	t.Helper()
	f := bind(t, configure)
	f.s.Start()
	return f
}

// close stops the server once; later calls are no-ops.
func (f *fixture) close() { f.closeOnce.Do(f.s.Close) }

func (f *fixture) sendUDP(t testing.TB, payload string) {
	t.Helper()
	conn, err := net.Dial("udp", f.addr)
	require.NoError(t, err)
	defer conn.Close()
	_, err = conn.Write([]byte(payload))
	require.NoError(t, err)
}

func (f *fixture) dialTCP(t testing.TB) net.Conn {
	t.Helper()
	conn, err := net.Dial("tcp", f.addr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// framing is the observable framing outcome: records handed to Ingest and the
// framing rejection counters.
type framing struct {
	records                []string
	oversize, unterminated uint64
}

func (f *fixture) framing() framing {
	return framing{
		records:      f.rec.snapshot(),
		oversize:     f.stats.Oversize.Load(),
		unterminated: f.stats.Unterminated.Load(),
	}
}

// waitFraming waits until the outcome reaches exactly want, then checks that
// nothing further arrives.
func (f *fixture) waitFraming(t testing.TB, want framing) {
	t.Helper()
	require.Eventually(t, func() bool {
		got := f.framing()
		return slices.Equal(got.records, want.records) && got.oversize == want.oversize &&
			got.unterminated == want.unterminated
	}, waitFor, time.Millisecond, "want %+v", want)
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, want, f.framing())
}

// freeAddress returns a loopback address currently free for both UDP and TCP.
func freeAddress(t testing.TB) string {
	t.Helper()
	for range 20 {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		addr := ln.Addr().String()
		pc, err := net.ListenPacket("udp", addr)
		_ = ln.Close()
		if err == nil {
			_ = pc.Close()
			return addr
		}
	}
	t.Fatal("no loopback port free for both UDP and TCP")
	return ""
}

func requireFree(t testing.TB, network, addr string) {
	t.Helper()
	var closer io.Closer
	var err error
	if network == "udp" {
		closer, err = net.ListenPacket(network, addr)
	} else {
		closer, err = net.Listen(network, addr)
	}
	require.NoError(t, err, "%s %s must be free", network, addr)
	require.NoError(t, closer.Close())
}
