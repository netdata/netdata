// SPDX-License-Identifier: GPL-3.0-or-later

package statsd

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

const (
	protocolUDP = "udp"
	protocolTCP = "tcp"
	// listenerErrorBackoff delays the next read or accept on a still-bound
	// listener after a temporary socket error.
	listenerErrorBackoff = 100 * time.Millisecond
)

// server owns the sockets and reader goroutines of one Run. close joins every
// goroutine, so Run returns only after all socket I/O has stopped.
type server struct {
	receiver  *receiver
	stats     *diagnostics
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

// listen acquires every configured listener. Any failure closes the listeners
// this attempt already bound; there is no fallback address or retry here.
func (c *Collector) listen(ctx context.Context) (*server, error) {
	s := &server{
		receiver:  c.receiver,
		stats:     c.diagnostics,
		now:       c.now,
		log:       c.Logger,
		maxRecord: c.maxRecord,
		maxConns:  c.MaxTCPConnections,
		conns:     make(map[net.Conn]struct{}),
		done:      make(chan struct{}),
		failed:    make(chan struct{}),
	}
	var lc net.ListenConfig
	for i, l := range c.Listeners {
		var err error
		switch l.Protocol {
		case protocolUDP:
			var conn net.PacketConn
			if conn, err = lc.ListenPacket(ctx, protocolUDP, l.Address); err == nil {
				s.udp = append(s.udp, conn)
			}
		case protocolTCP:
			var ln net.Listener
			if ln, err = lc.Listen(ctx, protocolTCP, l.Address); err == nil {
				s.tcp = append(s.tcp, ln)
			}
		}
		if err != nil {
			s.closeListeners()
			return nil, fmt.Errorf("listeners[%d]: %w", i, err)
		}
	}
	return s, nil
}

func (s *server) start() {
	s.wg.Add(len(s.udp) + len(s.tcp))
	for _, conn := range s.udp {
		go s.run(func() { s.readUDP(conn) })
	}
	for _, ln := range s.tcp {
		go s.run(func() { s.acceptTCP(ln) })
	}
}

// run executes one reader for this job. The framework recovers panics only in
// Run's goroutine, so a reader panic on untrusted input fails the receiver the
// same way instead of crashing the plugin process.
func (s *server) run(read func()) {
	defer s.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			if logger.Level.Enabled(slog.LevelDebug) {
				s.log.Errorf("STACK: %s", debug.Stack())
			}
			s.fail(fmt.Errorf("receiver panic: %v", r))
		}
	}()
	read()
}

func (s *server) close() {
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

func (s *server) closeListeners() {
	for _, conn := range s.udp {
		_ = conn.Close()
	}
	for _, ln := range s.tcp {
		_ = ln.Close()
	}
}

func (s *server) fail(err error) {
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
// whole receiver.
func (s *server) recoverable(err error, listener string) bool {
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

func (s *server) readUDP(conn net.PacketConn) {
	listener := "udp listener " + conn.LocalAddr().String()
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
// boundary also ends its final record. Datagrams are never joined.
func (s *server) datagram(payload []byte) {
	now := s.now()
	for len(payload) > 0 {
		line, rest, terminated := bytes.Cut(payload, []byte{'\n'})
		if terminated {
			line = bytes.TrimSuffix(line, []byte{'\r'})
		}
		s.record(line, now)
		payload = rest
	}
}

func (s *server) record(line []byte, now time.Time) {
	// Empty lines are not records. Rejections are counted by the receiver.
	if len(line) > 0 {
		_ = s.receiver.ingest(string(line), now)
	}
}

func (s *server) acceptTCP(ln net.Listener) {
	listener := "tcp listener " + ln.Addr().String()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if s.recoverable(err, listener) {
				continue
			}
			return
		}
		if s.track(conn) {
			go s.run(func() { s.readTCP(conn) })
		}
	}
}

// track admits a connection within the job-wide cap. Established clients keep
// their slots however quiet they are; a new client at capacity is refused.
func (s *server) track(conn net.Conn) bool {
	s.mu.Lock()
	closed := s.closed
	admitted := !closed && len(s.conns) < s.maxConns
	if admitted {
		s.conns[conn] = struct{}{}
		s.stats.tcpConnections.Store(int64(len(s.conns)))
		s.wg.Add(1)
	}
	s.mu.Unlock()
	if !admitted {
		_ = conn.Close()
		if !closed {
			s.receiver.reject(rejectConnectionLimit)
		}
	}
	return admitted
}

func (s *server) untrack(conn net.Conn) {
	s.mu.Lock()
	delete(s.conns, conn)
	s.stats.tcpConnections.Store(int64(len(s.conns)))
	s.mu.Unlock()
	_ = conn.Close()
}

// readTCP frames one client stream. Every record needs LF or CRLF, including
// the last: a fragment at EOF is rejected. A record over the bound is rejected
// and discarded through its newline, then reading resumes. Read errors and
// disconnects end only this connection.
func (s *server) readTCP(conn net.Conn) {
	defer s.untrack(conn)
	r := bufio.NewReaderSize(byteCounter{conn, &s.stats.tcpBytes}, s.maxRecord+len("\r\n"))
	discarding := false
	for {
		line, err := r.ReadSlice('\n')
		switch {
		case err == nil:
			if discarding {
				discarding = false
				continue
			}
			line = bytes.TrimSuffix(line[:len(line)-1], []byte{'\r'})
			if len(line) > s.maxRecord {
				s.receiver.reject(rejectOversize)
				continue
			}
			s.record(line, s.now())
		case errors.Is(err, bufio.ErrBufferFull):
			if !discarding {
				discarding = true
				s.receiver.reject(rejectOversize)
			}
		default:
			if len(line) > 0 && !discarding {
				s.receiver.reject(rejectUnterminated)
			}
			return
		}
	}
}

type byteCounter struct {
	r     io.Reader
	total *atomic.Uint64
}

func (c byteCounter) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.total.Add(uint64(n))
	return n, err
}
