// SPDX-License-Identifier: GPL-3.0-or-later

// Package server owns the StatsD job's sockets: acquisition, UDP and TCP newline
// framing, the TCP connection cap and listener failure handling. It hands every
// complete record to one ingest function and knows nothing about StatsD records
// beyond framing.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

const (
	// MaxRecordSize is the single framer-owned bound on one record payload,
	// excluding its LF/CRLF terminator. It exceeds the largest UDP payload.
	MaxRecordSize = 64 << 10
	// listenerErrorBackoff delays the next read or accept on a still-bound
	// listener after a temporary socket error.
	listenerErrorBackoff = 100 * time.Millisecond
)

// Endpoint is one socket to bind.
type Endpoint struct {
	Network string // "udp" or "tcp"
	Address string // host:port
	Name    string // identifies the endpoint in errors, such as "listeners[1]"
}

// Config describes one Server. Stats is caller-owned, so cumulative counters
// survive a new Server after a failed or retried run.
type Config struct {
	Endpoints []Endpoint
	MaxRecord int // record payload bound, excluding the terminator
	MaxConns  int // simultaneous TCP clients across every TCP endpoint
	Stats     *Stats
	Ingest    func(record string, now time.Time)
	Now       func() time.Time
	Log       *logger.Logger
}

// Stats are cumulative socket counters, updated by the reader goroutines.
type Stats struct {
	UDPBytes, TCPBytes atomic.Uint64
	TCPConnections     atomic.Int64  // open TCP clients
	TCPRefused         atomic.Uint64 // clients closed at MaxConns
	Oversize           atomic.Uint64 // records over MaxRecord, rejected whole
	Unterminated       atomic.Uint64 // TCP fragments without a newline at EOF
}

// Server owns the sockets and reader goroutines of one run. Close joins every
// goroutine, so the caller returns only after all socket I/O has stopped.
type Server struct {
	sink      func(record string, now time.Time)
	stats     *Stats
	now       func() time.Time
	log       *logger.Logger
	maxRecord int
	maxConns  int

	udp []net.PacketConn
	tcp []net.Listener

	mu     sync.Mutex
	conns  map[net.Conn]struct{}
	closed bool
	done   chan struct{} // closed when shutdown starts
	wg     sync.WaitGroup

	failOnce sync.Once
	failed   chan struct{}
	err      error
}

// Listen binds every endpoint in order. Any failure closes the sockets this
// attempt already bound; there is no fallback address or retry here.
func Listen(ctx context.Context, cfg Config) (*Server, error) {
	s := &Server{
		sink:      cfg.Ingest,
		stats:     cfg.Stats,
		now:       cfg.Now,
		log:       cfg.Log,
		maxRecord: cfg.MaxRecord,
		maxConns:  cfg.MaxConns,
		conns:     make(map[net.Conn]struct{}),
		done:      make(chan struct{}),
		failed:    make(chan struct{}),
	}
	var lc net.ListenConfig
	for _, e := range cfg.Endpoints {
		var err error
		switch e.Network {
		case "udp":
			var conn net.PacketConn
			if conn, err = lc.ListenPacket(ctx, e.Network, e.Address); err == nil {
				s.udp = append(s.udp, conn)
			}
		case "tcp":
			var ln net.Listener
			if ln, err = lc.Listen(ctx, e.Network, e.Address); err == nil {
				s.tcp = append(s.tcp, ln)
			}
		default:
			err = fmt.Errorf("unsupported network %q", e.Network)
		}
		if err != nil {
			s.closeListeners()
			return nil, fmt.Errorf("%s: %w", e.Name, err)
		}
	}
	return s, nil
}

// Failed is closed when a listener is lost or a reader panics.
func (s *Server) Failed() <-chan struct{} { return s.failed }

// Err returns the failure that closed Failed, or nil. Call it after Close.
func (s *Server) Err() error { return s.err }

// Start runs one reader per socket.
func (s *Server) Start() {
	s.wg.Add(len(s.udp) + len(s.tcp))
	for _, conn := range s.udp {
		listener := "udp listener " + conn.LocalAddr().String()
		go s.guard(listener, func() { s.readUDP(conn, listener) })
	}
	for _, ln := range s.tcp {
		listener := "tcp listener " + ln.Addr().String()
		go s.guard(listener, func() { s.acceptTCP(ln, listener) })
	}
}

// guard runs one named reader for this job. The framework recovers panics only
// in the job's Run goroutine, so a reader panic, including one raised by the
// ingest function, fails the server instead of crashing the plugin process.
func (s *Server) guard(reader string, read func()) {
	defer s.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			if logger.Level.Enabled(slog.LevelDebug) {
				s.log.Errorf("%s: STACK: %s", reader, debug.Stack())
			}
			err, ok := r.(error)
			if !ok {
				err = fmt.Errorf("%v", r)
			}
			s.fail(fmt.Errorf("%s: receiver panic: %w", reader, err))
		}
	}()
	read()
}

// Close stops accepting, closes every socket and client, and joins every reader.
func (s *Server) Close() {
	s.mu.Lock()
	s.closed = true
	close(s.done)
	s.closeListeners()
	for conn := range s.conns {
		_ = conn.Close()
	}
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *Server) closeListeners() {
	for _, conn := range s.udp {
		_ = conn.Close()
	}
	for _, ln := range s.tcp {
		_ = ln.Close()
	}
}

func (s *Server) fail(err error) {
	s.failOnce.Do(func() {
		s.err = err
		close(s.failed)
	})
}

// recoverable reports whether a socket error leaves the listener usable.
// Errors reporting Temporary() (descriptor exhaustion, timeouts) back off on
// the same bound socket; the Go runtime already retries interrupted calls and
// aborted connections. Any other error while the job is not stopping, including
// an unexpectedly closed socket, is permanent listener loss and fails the
// whole server.
func (s *Server) recoverable(err error, listener string) bool {
	select {
	case <-s.done:
		return false
	default:
	}
	var temporary interface{ Temporary() bool }
	if errors.As(err, &temporary) && temporary.Temporary() {
		cause := err
		var op *net.OpError
		if errors.As(err, &op) {
			cause = op.Err
		}
		s.log.Limit("statsd:listener-temporary-error", 1, time.Minute).
			Warningf("%s: temporary error, continuing: %v", listener, cause)
		select {
		case <-s.done:
			return false
		case <-time.After(listenerErrorBackoff):
			return true
		}
	}
	s.fail(fmt.Errorf("%s: %w", listener, err))
	return false
}

// ingest hands one framed record to the ingest function. Empty lines are not
// records.
func (s *Server) ingest(line string, now time.Time) {
	if line != "" {
		s.sink(line, now)
	}
}
