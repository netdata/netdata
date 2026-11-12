//go:build windows

package windows

import (
	"errors"
	"syscall"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/internal/monotime"
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

	s.initReceiveFns()
	s.recvControl = timeoutMs != 0 || abortCh != nil
	s.recvWait = newReceiveWait(timeoutMs, abortCh)

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
		Recv:                    s.recvPacketFn,
		EnsurePacketScratch:     ensurePipeScratchBuf,
		IsRecvDisconnect:        func(err error) bool { return errors.Is(err, ErrDisconnected) },
		PropagateRecvError: func(err error) bool {
			return errors.Is(err, ErrTimeout) || errors.Is(err, ErrAborted)
		},
		FailAllInflight:  s.failInflights,
		ErrLimitExceeded: func(msg string) error { return wrapErr(ErrLimitExceeded, msg) },
		ErrProtocol:      func(msg string) error { return wrapErr(ErrProtocol, msg) },
		ErrChunk:         func(msg string) error { return wrapErr(ErrChunk, msg) },
		ErrUnknownMsgID:  func(msg string) error { return wrapErr(ErrUnknownMsgID, msg) },
		ErrRecv:          func(msg string) error { return wrapErr(ErrRecv, msg) },
	}, buf)
}

// initReceiveFns builds the session's receive closures once. They read the
// call's state from the session (recvControl, recvWait).
func (s *Session) initReceiveFns() {
	if s.recvPacketFn != nil {
		return
	}
	s.recvPacketFn = func(dst []byte) (int, error) {
		if s.pending.queued() {
			return s.pending.pop(dst)
		}
		if s.recvControl {
			return s.rawRecvWithTimeout(dst, s.recvWait)
		}
		return s.readPacket(dst)
	}
	s.failInflights = s.failAllInflight
}

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

	remaining := time.Duration(w.deadline - monotime.Now())
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
		return s.readPacket(buf)
	}
}
