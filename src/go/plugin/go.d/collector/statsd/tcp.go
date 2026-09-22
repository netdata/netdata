// SPDX-License-Identifier: GPL-3.0-or-later

package statsd

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
	"sync/atomic"
)

// acceptTCP accepts clients until the listener closes or fails permanently.
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
			go s.guard(func() { s.readTCP(conn) })
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
			s.ingest(line, s.now())
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

// byteCounter counts bytes read from a TCP client for diagnostics.
type byteCounter struct {
	r     io.Reader
	total *atomic.Uint64
}

func (c byteCounter) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.total.Add(uint64(n))
	return n, err
}
