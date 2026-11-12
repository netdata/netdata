//go:build unix

package posix

import (
	"errors"
	"syscall"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/internal/monotime"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/internal/syspoll"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/internal/framing"
)

const receiveAbortPollMs = 100

type receiveWait struct {
	infinite bool
	deadline int64 // monotime nanoseconds
	abortCh  <-chan struct{}
}

func newReceiveWait(timeoutMs uint32, abortCh <-chan struct{}) receiveWait {
	w := receiveWait{
		infinite: timeoutMs == 0,
		abortCh:  abortCh,
	}
	if !w.infinite {
		w.deadline = monotime.After(time.Duration(timeoutMs) * time.Millisecond)
	}
	return w
}

func (w receiveWait) waitMs() (int, error) {
	if w.abortCh != nil {
		select {
		case <-w.abortCh:
			return 0, ErrAborted
		default:
		}
	}

	waitMs := receiveAbortPollMs
	if w.infinite {
		return waitMs, nil
	}

	remaining := time.Duration(w.deadline - monotime.Now())
	if remaining <= 0 {
		return 0, ErrTimeout
	}
	if remaining < time.Duration(waitMs)*time.Millisecond {
		waitMs = int((remaining + time.Millisecond - 1) / time.Millisecond)
		if waitMs == 0 {
			waitMs = 1
		}
	}
	return waitMs, nil
}

func pollReadableForReceive(fd int, wait receiveWait) error {
	const maxInt32 = 1<<31 - 1
	if fd < 0 || fd > maxInt32 {
		return wrapErr(ErrBadParam, "fd out of range")
	}

	for {
		waitMs, err := wait.waitMs()
		if err != nil {
			return err
		}

		pfd := syspoll.PollFd{
			Fd:     int32(fd), // #nosec G115 -- fd is checked against int32 range above.
			Events: syspoll.In,
		}
		n, errno := syspoll.Poll(&pfd, waitMs)
		if n < 0 {
			if errno == syscall.EINTR {
				continue
			}
			return wrapErr(ErrRecv, errno.Error())
		}
		if n == 0 {
			continue
		}
		if pfd.Revents&syspoll.In != 0 {
			return nil
		}
		if pfd.Revents&(syspoll.Err|syspoll.Hup|syspoll.Nval) != 0 {
			return nil
		}
	}
}

func rawRecvWithTimeout(fd int, buf []byte, wait receiveWait) (int, error) {
	if err := pollReadableForReceive(fd, wait); err != nil {
		return 0, err
	}
	return rawRecv(fd, buf)
}

// Receive reads one logical message. Blocks until a complete message
// arrives. buf is a caller-provided scratch buffer for the first packet.
// On success, returns the header and a payload view valid until the next
// Receive call on this session.
func (s *Session) Receive(buf []byte) (protocol.Header, []byte, error) {
	return s.ReceiveTimeout(buf, 0, nil)
}

func (s *Session) ReceiveTimeout(buf []byte, timeoutMs uint32, abortCh <-chan struct{}) (protocol.Header, []byte, error) {
	if s.fd < 0 {
		return protocol.Header{}, nil, wrapErr(ErrBadParam, "session closed")
	}
	// Rejected before any byte is read, so the session stays usable.
	if len(buf) < protocol.HeaderSize {
		return protocol.Header{}, nil, wrapErr(ErrBadParam, "receive buffer too small")
	}

	s.initReceiveFns()
	s.recvControl = timeoutMs != 0 || abortCh != nil
	s.recvWait = newReceiveWait(timeoutMs, abortCh)
	if streamTransport {
		return s.receiveStream(buf)
	}
	return s.receiveFraming(buf, s.recvPacketFn, false)
}

// initReceiveFns builds the session's receive closures once. They read the
// call's state from the session (recvControl, recvWait, streamFirst).
func (s *Session) initReceiveFns() {
	if s.recvPacketFn != nil {
		return
	}
	s.recvPacketFn = func(dst []byte) (int, error) {
		if s.pending.queued() {
			return s.pending.popPacket(dst), nil
		}
		if s.recvControl {
			return rawRecvWithTimeout(s.fd, dst, s.recvWait)
		}
		return rawRecv(s.fd, dst)
	}
	s.recvStreamFn = func(dst []byte) (int, error) {
		var waitp *receiveWait
		if s.recvControl {
			waitp = &s.recvWait
		}
		maxPayload, maxItems := s.MaxResponsePayloadBytes, s.MaxResponseBatchItems
		if s.role == RoleServer {
			maxPayload, maxItems = s.MaxRequestPayloadBytes, s.MaxRequestBatchItems
		}
		n, err := streamRecvSessionPacket(s.fd, &s.pending, dst, s.streamFirst, s.PacketSize, maxPayload, maxItems, waitp, &s.streamConsumed)
		s.streamFirst = false
		return n, err
	}
	s.failInflights = s.failAllInflight
}

// errRecvFailed is what every receive returns after a stream receive failed
// inside a message.
var errRecvFailed = wrapErr(ErrRecv, "an earlier receive failed inside a message")

// receiveStream receives on a stream socket. A failure before the first byte
// (a timeout or abort, also one that fires after the wait below) leaves the
// stream on a packet boundary. A failure after it does not: the rest of the
// message is still queued, so every later receive fails.
// The session records that instead of shutting the read side, whose effect
// differs by OS (FreeBSD 15 fails the peer's sends, FreeBSD 14 still returns
// queued bytes). Sends stay valid; they always write whole messages.
func (s *Session) receiveStream(buf []byte) (protocol.Header, []byte, error) {
	if s.recvFailed {
		return protocol.Header{}, nil, errRecvFailed
	}
	if s.recvControl && !s.pending.queued() {
		if err := pollReadableForReceive(s.fd, s.recvWait); err != nil {
			return protocol.Header{}, nil, err
		}
	}
	s.streamFirst = true
	s.streamConsumed = false
	hdr, payload, err := s.receiveFraming(buf, s.recvStreamFn, true)
	if err != nil && s.streamConsumed {
		s.recvFailed = true
		// Queued bytes are part of the abandoned stream too, and no response
		// for a request in flight can arrive any more.
		s.pending = pendingQueue{}
		s.failAllInflight()
	}
	return hdr, payload, err
}

func propagateWaitError(err error) bool {
	return errors.Is(err, ErrTimeout) || errors.Is(err, ErrAborted)
}

func propagateStreamError(err error) bool {
	return propagateWaitError(err) || errors.Is(err, ErrChunk)
}

// propagateRecvError picks a top-level function, so a receive allocates no
// closure for it.
func propagateRecvError(stream bool) func(error) bool {
	if stream {
		return propagateStreamError
	}
	return propagateWaitError
}

// receiveFraming runs the shared message framing over recv. A stream's
// continuation errors keep their class (a truncated chunk is ErrChunk), as
// a SEQPACKET continuation's framing errors do.
func (s *Session) receiveFraming(buf []byte, recv func([]byte) (int, error), stream bool) (protocol.Header, []byte, error) {
	return framing.SessionReceive(framing.SessionReceiveConfig{
		RoleServer:              s.role == RoleServer,
		PacketSize:              s.PacketSize,
		MaxRequestPayloadBytes:  s.MaxRequestPayloadBytes,
		MaxRequestBatchItems:    s.MaxRequestBatchItems,
		MaxResponsePayloadBytes: s.MaxResponsePayloadBytes,
		MaxResponseBatchItems:   s.MaxResponseBatchItems,
		InflightIDs:             s.inflightIDs,
		RecvBuf:                 &s.recvBuf,
		PacketBuf:               &s.pktBuf,
		Recv:                    recv,
		EnsurePacketScratch:     ensureScratchBuf,
		IsRecvDisconnect:        func(err error) bool { return errors.Is(err, ErrRecv) },
		PropagateRecvError:      propagateRecvError(stream),
		FailAllInflight:         s.failInflights,
		ErrLimitExceeded:        func(msg string) error { return wrapErr(ErrLimitExceeded, msg) },
		ErrProtocol:             func(msg string) error { return wrapErr(ErrProtocol, msg) },
		ErrChunk:                func(msg string) error { return wrapErr(ErrChunk, msg) },
		ErrUnknownMsgID:         func(msg string) error { return wrapErr(ErrUnknownMsgID, msg) },
		ErrRecv:                 func(msg string) error { return wrapErr(ErrRecv, msg) },
	}, buf)
}
