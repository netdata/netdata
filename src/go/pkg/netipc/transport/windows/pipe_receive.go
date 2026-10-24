//go:build windows

package windows

import (
	"errors"
	"syscall"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/internal/framing"
)

const receiveAbortPollMs uint32 = 100

// Receive reads one logical message. buf is a scratch buffer.
func (s *Session) Receive(buf []byte) (protocol.Header, []byte, error) {
	return s.ReceiveTimeout(buf, 0, nil)
}

func (s *Session) ReceiveTimeout(buf []byte, timeoutMs uint32, abortCh <-chan struct{}) (protocol.Header, []byte, error) {
	if s.handle == syscall.InvalidHandle {
		return protocol.Header{}, nil, wrapErr(ErrBadParam, "session closed")
	}

	recv := func(dst []byte) (int, error) { return rawRecv(s.handle, dst) }
	if timeoutMs != 0 || abortCh != nil {
		wait := newReceiveWait(timeoutMs, abortCh)
		recv = func(dst []byte) (int, error) { return s.rawRecvWithTimeout(dst, wait) }
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
		EnsurePacketScratch:     ensurePipeScratchBuf,
		IsRecvDisconnect:        func(err error) bool { return errors.Is(err, ErrDisconnected) },
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

type receiveWait struct {
	infinite bool
	deadline time.Time
	abortCh  <-chan struct{}
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

func (w receiveWait) waitMs() (uint32, error) {
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
	return boundedReceiveWaitMs(remaining, waitMs), nil
}

func boundedReceiveWaitMs(remaining time.Duration, pollCapMs uint32) uint32 {
	if remaining <= 0 {
		return 0
	}
	if remaining >= time.Duration(pollCapMs)*time.Millisecond {
		return pollCapMs
	}
	waitMs := uint32((remaining + time.Millisecond - 1) / time.Millisecond) // #nosec G115 -- remaining is positive and below pollCapMs here.
	if waitMs == 0 {
		return 1
	}
	return waitMs
}

func (s *Session) rawRecvWithTimeout(buf []byte, wait receiveWait) (int, error) {
	for {
		waitMs, err := wait.waitMs()
		if err != nil {
			return 0, err
		}

		ready, err := s.WaitReadable(waitMs)
		if err != nil {
			return 0, err
		}
		if !ready {
			continue
		}
		return rawRecv(s.handle, buf)
	}
}
