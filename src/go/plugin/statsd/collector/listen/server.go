// SPDX-License-Identifier: GPL-3.0-or-later

package listen

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"runtime/debug"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

const (
	// maxRecordSize is the single framer-owned bound on one record payload,
	// excluding its LF/CRLF terminator. It exceeds the largest UDP payload.
	maxRecordSize = 64 << 10
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

func (c *Collector) run(ctx context.Context, ready func()) error {
	if c.receiver == nil {
		return errNotInitialized
	}
	if ctx.Err() != nil {
		return nil
	}
	s, err := c.listen(ctx)
	if err != nil {
		return err
	}
	return c.serve(ctx, s, ready)
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

// serve owns an acquired server. Readiness follows successful acquisition, not
// traffic. On cancellation or listener loss, admission stops before sockets
// close, so a failed receiver cannot publish held or empty intervals.
func (c *Collector) serve(ctx context.Context, s *server, ready func()) error {
	c.receiver.start()
	s.start()
	ready()
	select {
	case <-ctx.Done():
	case <-s.failed:
	}
	c.receiver.stop()
	s.close()
	return s.err
}

func (s *server) start() {
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
// in Run's goroutine, so a reader panic on untrusted input fails the receiver
// the same way instead of crashing the plugin process.
func (s *server) guard(reader string, read func()) {
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

// ingest hands one framed record to the receiver, which counts rejections.
// Empty lines are not records.
func (s *server) ingest(line string, now time.Time) {
	if line != "" {
		_ = s.receiver.ingest(line, now)
	}
}
