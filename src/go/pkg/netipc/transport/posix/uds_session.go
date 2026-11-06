//go:build unix

package posix

import (
	"encoding/binary"
	"syscall"
)

// Role distinguishes client vs server sessions.
type Role int

const (
	RoleClient Role = 1
	RoleServer Role = 2
)

// ClientConfig configures a client connection.
type ClientConfig struct {
	SupportedProfiles       uint32
	PreferredProfiles       uint32
	MaxRequestPayloadBytes  uint32 // 0 = use default (1024)
	MaxRequestBatchItems    uint32 // 0 = use default (1)
	MaxResponsePayloadBytes uint32
	MaxResponseBatchItems   uint32
	AuthToken               uint64
	PacketSize              uint32 // 0 = kernel limit; capped at SO_SNDBUF-32 and 256 KiB
}

// ServerConfig configures a listener and its accepted sessions.
type ServerConfig struct {
	SupportedProfiles       uint32
	PreferredProfiles       uint32
	MaxRequestPayloadBytes  uint32
	MaxRequestBatchItems    uint32
	MaxResponsePayloadBytes uint32
	MaxResponseBatchItems   uint32
	AuthToken               uint64
	PacketSize              uint32 // 0 = kernel limit; capped at SO_SNDBUF-32 and 256 KiB
	Backlog                 int    // 0 = default (16)
}

// Session is a connected UDS SEQPACKET session (client or server side).
type Session struct {
	fd   int
	role Role

	MaxRequestPayloadBytes  uint32
	MaxRequestBatchItems    uint32
	MaxResponsePayloadBytes uint32
	MaxResponseBatchItems   uint32
	PacketSize              uint32
	SelectedProfile         uint32
	SessionID               uint64

	recvBuf []byte
	pktBuf  []byte

	inflightIDs map[uint64]struct{}

	// Inbound packets read while a pipelined send waited for buffer space.
	pending pendingQueue
	// Queue bound of the send in progress; 0 sends with a blocking call.
	progressBound int
	// Stream baseline: a receive failed after reading part of a message, so
	// the stream is off a packet boundary and every later receive fails.
	recvFailed bool

	// Receive state of the call in progress, read by closures built once
	// per session (receiveFns), so a receive allocates nothing.
	recvWait      receiveWait
	recvControl   bool
	streamFirst   bool
	recvPacketFn  func([]byte) (int, error)
	recvStreamFn  func([]byte) (int, error)
	failInflights func()
}

// pendingQueue holds inbound bytes a pipelined client send read while it
// waited for buffer space; receive consumes them before reading the socket.
// SEQPACKET stores each datagram as a 4-byte length record followed by its
// bytes; the stream stores raw bytes.
type pendingQueue struct {
	buf  []byte
	head int
}

func pendingRecordLen() int {
	if streamTransport {
		return 0
	}
	return 4
}

func (q *pendingQueue) queued() bool {
	return q != nil && q.head < len(q.buf)
}

func (q *pendingQueue) consumed() {
	if q.head == len(q.buf) {
		q.buf = q.buf[:0]
		q.head = 0
	}
}

// drain reads without blocking until the socket has nothing more to read,
// EOF or an error (both set *eof). Queued bytes beyond bound fail with
// ErrProtocol. A datagram larger than readLen is stored truncated, and the
// receive path rejects it.
func (q *pendingQueue) drain(fd, readLen, bound int, eof *bool) error {
	if q.head > 0 {
		n := copy(q.buf, q.buf[q.head:])
		q.buf = q.buf[:n]
		q.head = 0
	}
	rec := pendingRecordLen()
	for {
		start := len(q.buf)
		q.buf = append(q.buf, make([]byte, rec+readLen)...)
		n, _, _, _, err := syscall.Recvmsg(fd, q.buf[start+rec:], nil, syscall.MSG_DONTWAIT)
		if err != nil || n <= 0 {
			q.buf = q.buf[:start]
			switch {
			case err == syscall.EINTR:
				continue
			case err == syscall.EAGAIN || err == syscall.EWOULDBLOCK:
			default:
				// EOF or a socket error: the retried send reports it.
				*eof = true
			}
			return nil
		}
		if rec > 0 {
			binary.NativeEndian.PutUint32(q.buf[start:], uint32(n)) // #nosec G115 -- n <= readLen, a uint32 packet size.
		}
		q.buf = q.buf[:start+rec+n]
		if len(q.buf) > bound {
			return wrapErr(ErrProtocol, "inbound bytes exceed the in-flight responses")
		}
	}
}

// popPacket pops one queued SEQPACKET datagram into dst, truncating it to
// len(dst) as recv does. It returns the bytes copied.
func (q *pendingQueue) popPacket(dst []byte) int {
	rec := int(binary.NativeEndian.Uint32(q.buf[q.head:]))
	start := q.head + 4
	n := copy(dst, q.buf[start:start+rec])
	q.head = start + rec
	q.consumed()
	return n
}

// take copies up to len(dst) queued stream bytes and returns the count.
func (q *pendingQueue) take(dst []byte) int {
	if !q.queued() {
		return 0
	}
	n := copy(dst, q.buf[q.head:])
	q.head += n
	q.consumed()
	return n
}

func (s *Session) failAllInflight() {
	if s.role != RoleClient || len(s.inflightIDs) == 0 {
		return
	}
	clear(s.inflightIDs)
}

// Fd returns the raw file descriptor for poll/epoll integration. A
// pipelined send can read responses into the session's pending queue, where
// the fd no longer shows them: check HasPending before waiting on the fd.
func (s *Session) Fd() int {
	return s.fd
}

// HasPending reports whether the next Receive can return data read earlier
// by a pipelined send, without the fd becoming readable.
func (s *Session) HasPending() bool {
	return !s.recvFailed && s.pending.queued()
}

// Role returns the session role.
func (s *Session) Role() Role {
	return s.role
}

// Close closes the session and releases resources.
func (s *Session) Close() {
	if s.fd >= 0 {
		_ = syscall.Close(s.fd)
		s.fd = -1
	}
	s.recvBuf = nil
	s.pktBuf = nil
	s.pending = pendingQueue{}
	s.failAllInflight()
}

// Connect establishes a session to a server at {runDir}/{serviceName}.sock.
// Performs the full handshake. Blocks until connected + handshake done.
func Connect(runDir, serviceName string, config *ClientConfig) (*Session, error) {
	return ConnectTimeout(runDir, serviceName, config, 0, nil)
}

// ConnectTimeout is Connect bounded by timeoutMs: connect and the wait for
// HELLO_ACK together end with ErrTimeout after timeoutMs (0 = no deadline),
// and with ErrAborted once abortCh is closed (nil = no abort).
func ConnectTimeout(runDir, serviceName string, config *ClientConfig, timeoutMs uint32, abortCh <-chan struct{}) (*Session, error) {
	wait := newReceiveWait(timeoutMs, abortCh)
	path, err := buildSocketPath(runDir, serviceName)
	if err != nil {
		return nil, err
	}

	fd, err := udsSocket()
	if err != nil {
		return nil, wrapErr(ErrSocket, err.Error())
	}

	session, herr := connectAndHandshake(fd, path, config, wait)
	if herr != nil {
		_ = syscall.Close(fd)
		return nil, herr
	}
	return session, nil
}
