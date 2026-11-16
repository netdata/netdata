//go:build unix

package posix

import (
	"errors"
	"fmt"
	"math"
	"syscall"
	"unsafe"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/internal/syspoll"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/internal/framing"
)

// Send sends one logical message. The caller fills Kind, Code, Flags,
// ItemCount, MessageID in hdr; this function sets Magic/Version/
// HeaderLen/PayloadLen. If the total message exceeds PacketSize,
// it is chunked transparently.
func (s *Session) Send(hdr *protocol.Header, payload []byte) error {
	return s.SendTimeout(hdr, payload, 0, nil)
}

// SendTimeout is Send with an optional deadline and abort channel for a
// client session. timeoutMs == 0 waits without a deadline. While the socket
// lacks buffer space the send reads the peer's bytes as a pipelined send
// does. If the deadline passes or abortCh is closed before the message is
// sent, it returns ErrTimeout or ErrAborted and shuts the session down,
// since part of the message may be on the wire.
func (s *Session) SendTimeout(hdr *protocol.Header, payload []byte, timeoutMs uint32, abortCh <-chan struct{}) error {
	if s.fd < 0 {
		return wrapErr(ErrBadParam, "session closed")
	}

	// A client send with a deadline or abort, or with another request in
	// flight (a pipelining client can deadlock with a server blocked sending
	// to it), waits in the progress loop; anything else keeps the plain
	// blocking send.
	s.sendBounded = s.role == RoleClient && (timeoutMs != 0 || abortCh != nil)
	if s.sendBounded {
		s.sendWait = newReceiveWait(timeoutMs, abortCh)
	}
	inflight := len(s.inflightIDs)
	if s.role == RoleClient && hdr.Kind == protocol.KindRequest {
		inflight++
	}
	s.progressBound = 0
	if s.role == RoleClient && (inflight > 1 || s.sendBounded) {
		// A bounded send of an untracked message can run with nothing in
		// flight; it still gets one response's room.
		s.progressBound = pendingBound(s.MaxResponsePayloadBytes, s.PacketSize, max(inflight, 1))
	}

	return framing.SessionSend(framing.SessionSendConfig{
		RoleClient:       s.role == RoleClient,
		PacketSize:       s.PacketSize,
		InflightIDs:      &s.inflightIDs,
		FailAllInflight:  s.failAllInflight,
		IsSendDisconnect: func(err error) bool { return errors.Is(err, ErrSend) },
		SendFirstPacket: func(packetHdr *protocol.Header, packetPayload []byte, _ int) error {
			var hdrBuf [protocol.HeaderSize]byte
			packetHdr.Encode(hdrBuf[:])
			return s.sendSessionPacket(hdrBuf[:], packetPayload)
		},
		SendChunk: func(chk protocol.ChunkHeader, chunkPayload []byte) error {
			var chkBuf [protocol.HeaderSize]byte
			chk.Encode(chkBuf[:])
			return s.sendSessionPacket(chkBuf[:], chunkPayload)
		},
		ErrLimitExceeded:  func(msg string) error { return wrapErr(ErrLimitExceeded, msg) },
		ErrDuplicateMsgID: func(msg string) error { return wrapErr(ErrDuplicateMsgID, msg) },
		ErrBadParam:       func(msg string) error { return wrapErr(ErrBadParam, msg) },
	}, hdr, payload)
}

// pendingBound is the wire bytes inflight responses can occupy: the pending
// queue bound.
// It saturates at math.MaxInt (32-bit targets), as C and Rust do.
func pendingBound(maxResponsePayload, packetSize uint32, inflight int) int {
	payload := uint64(maxResponsePayload)
	budget := max(uint64(packetSize), protocol.HeaderSize+1) - protocol.HeaderSize
	packets := uint64(1)
	if payload > 0 {
		packets = 1 + (payload-1)/budget
	}
	// payload and packets are below 2^32, so the per-response size fits.
	perResponse := payload + packets*uint64(protocol.HeaderSize+pendingRecordLen())
	if perResponse > uint64(math.MaxInt)/uint64(inflight) {
		return math.MaxInt
	}
	return int(perResponse * uint64(inflight)) // #nosec G115 -- checked against math.MaxInt above.
}

func (s *Session) sendSessionPacket(hdr, payload []byte) error {
	if s.progressBound == 0 {
		return sendPacket(s.fd, hdr, payload)
	}
	// macOS ignores MSG_DONTWAIT on a stream send that lacks buffer space
	// and blocks, so on the stream baseline (macOS, FreeBSD) the descriptor
	// is made non-blocking for the send instead. Sessions are blocking
	// otherwise.
	if streamTransport {
		if err := syscall.SetNonblock(s.fd, true); err != nil {
			return wrapErr(ErrSend, err.Error())
		}
	}
	err := s.sendWithProgress(hdr, payload)
	if streamTransport {
		// A descriptor left non-blocking would break later blocking calls;
		// ErrSend also fails every in-flight request.
		if rerr := syscall.SetNonblock(s.fd, false); rerr != nil {
			_ = syscall.Shutdown(s.fd, syscall.SHUT_RDWR)
			return wrapErr(ErrSend, rerr.Error())
		}
	}
	if errors.Is(err, ErrProtocol) || errors.Is(err, ErrTimeout) || errors.Is(err, ErrAborted) {
		// Inbound bytes were dropped, or the message stopped part way: the
		// session cannot continue.
		_ = syscall.Shutdown(s.fd, syscall.SHUT_RDWR)
		s.failAllInflight()
	}
	return err
}

// sendWithProgress sends one packet without blocking; while the socket is
// full, it reads what the peer sent into the pending queue, so both peers
// keep making progress. Without sendWait it waits as long as a blocking send
// would; with one it ends at the deadline or abort.
func (s *Session) sendWithProgress(hdr, payload []byte) error {
	const maxInt32 = 1<<31 - 1
	if s.fd < 0 || s.fd > maxInt32 {
		return wrapErr(ErrBadParam, "fd out of range")
	}
	parts := [2][]byte{hdr, payload}
	first := 0
	// After a failed stream receive the inbound bytes belong to an abandoned
	// message: never queue them.
	eof := s.recvFailed
	for {
		var iov [2]syscall.Iovec
		msg := syscall.Msghdr{Iov: &iov[0]}
		count := 0
		for _, part := range parts[first:] {
			if len(part) > 0 {
				iov[count].Base = unsafe.SliceData(part) // #nosec G103 -- sendmsg iovec needs the backing byte-slice pointer.
				iov[count].SetLen(len(part))
				count++
			}
		}
		// Msghdr.Iovlen differs in type across platforms.
		msg.Iovlen = 1
		if count == 2 {
			msg.Iovlen = 2
		}

		n, errno := rawSendmsg(s.fd, &msg, sendFlags|syscall.MSG_DONTWAIT)
		if errno == 0 {
			if !streamTransport {
				// A SEQPACKET send moves the whole packet or nothing.
				if int(n) != len(hdr)+len(payload) {
					return wrapErr(ErrSend, fmt.Sprintf("short write: %d/%d", n, len(hdr)+len(payload)))
				}
				return nil
			}
			if n == 0 {
				return wrapErr(ErrSend, "stream write moved no data")
			}
			left := int(n)
			for first < len(parts) && left >= len(parts[first]) {
				left -= len(parts[first])
				first++
			}
			if first == len(parts) {
				return nil
			}
			parts[first] = parts[first][left:]
			continue
		}
		if errno == syscall.EINTR {
			continue
		}
		if errno != syscall.EAGAIN && errno != syscall.EWOULDBLOCK {
			return wrapErr(ErrSend, errno.Error())
		}

		// After EOF only writability matters; the retried send reports a
		// closed peer.
		pfd := syspoll.PollFd{
			Fd:     int32(s.fd), // #nosec G115 -- fd is checked against int32 range above.
			Events: syspoll.Out,
		}
		if !eof {
			pfd.Events |= syspoll.In
		}
		waitMs := -1
		if s.sendBounded {
			ms, werr := s.sendWait.waitMs()
			if werr != nil {
				return werr
			}
			waitMs = ms
		}
		rc, perr := syspoll.Poll(&pfd, waitMs)
		if rc < 0 {
			if perr == syscall.EINTR {
				continue
			}
			return wrapErr(ErrSend, perr.Error())
		}
		if rc == 0 {
			// waitMs() reports the deadline and the abort channel.
			continue
		}
		if !eof && pfd.Revents&(syspoll.In|syspoll.Hup|syspoll.Err) != 0 {
			if err := s.pending.drain(s.fd, int(s.PacketSize), s.progressBound, &eof); err != nil {
				return err
			}
		}
	}
}
