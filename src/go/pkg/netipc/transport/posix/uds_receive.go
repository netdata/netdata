//go:build unix

package posix

import (
	"errors"
	"syscall"
	"time"
	"unsafe"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/internal/framing"
)

const receiveAbortPollMs = 100

const (
	_pollIn   = 0x0001
	_pollErr  = 0x0008
	_pollHup  = 0x0010
	_pollNval = 0x0020
)

type receiveWait struct {
	infinite bool
	deadline time.Time
	abortCh  <-chan struct{}
}

type receivePollFd struct {
	fd      int32
	events  int16
	revents int16
}

func newReceiveWait(timeoutMs uint32, abortCh <-chan struct{}) receiveWait {
	w := receiveWait{
		infinite: timeoutMs == 0,
		abortCh:  abortCh,
	}
	if !w.infinite {
		w.deadline = time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
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

	remaining := time.Until(w.deadline)
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

		pfd := receivePollFd{
			fd:     int32(fd), // #nosec G115 -- fd is checked against int32 range above.
			events: _pollIn,
		}
		r, _, errno := syscall.Syscall(
			syscall.SYS_POLL,
			uintptr(unsafe.Pointer(&pfd)), // #nosec G103 -- raw poll syscall requires a pollfd pointer.
			1,
			uintptr(waitMs),
		)
		n := int(r)
		if n < 0 {
			if errno == syscall.EINTR {
				continue
			}
			return wrapErr(ErrRecv, errno.Error())
		}
		if n == 0 {
			continue
		}
		if pfd.revents&_pollIn != 0 {
			return nil
		}
		if pfd.revents&(_pollErr|_pollHup|_pollNval) != 0 {
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

	recv := func(dst []byte) (int, error) { return rawRecv(s.fd, dst) }
	if timeoutMs != 0 || abortCh != nil {
		wait := newReceiveWait(timeoutMs, abortCh)
		recv = func(dst []byte) (int, error) { return rawRecvWithTimeout(s.fd, dst, wait) }
	}

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
		PropagateRecvError: func(err error) bool {
			return errors.Is(err, ErrTimeout) || errors.Is(err, ErrAborted)
		},
		FailAllInflight:  s.failAllInflight,
		ErrLimitExceeded: func(msg string) error { return wrapErr(ErrLimitExceeded, msg) },
		ErrProtocol:      func(msg string) error { return wrapErr(ErrProtocol, msg) },
		ErrChunk:         func(msg string) error { return wrapErr(ErrChunk, msg) },
		ErrUnknownMsgID:  func(msg string) error { return wrapErr(ErrUnknownMsgID, msg) },
		ErrRecv:          func(msg string) error { return wrapErr(ErrRecv, msg) },
	}, buf)
}
