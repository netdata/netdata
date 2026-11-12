//go:build unix

package posix

import (
	"encoding/binary"
	"errors"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

// Stream-socket packet I/O, the macOS and FreeBSD baseline (macOS has no
// AF_UNIX SOCK_SEQPACKET; FreeBSD 15 SEQPACKET keeps record boundaries only
// with MSG_EOR). A packet is the 32-byte header plus the bytes it declares,
// so the bytes on the wire match the SEQPACKET transport. Built on every
// POSIX system so its tests run everywhere; only macOS and FreeBSD sessions
// use it.

// streamTestIO, when set by tests, runs before each stream read or write of
// n bytes: it returns how many bytes the call may move and whether it fails
// with EINTR first, as a stream socket and a signal may make it.
var streamTestIO atomic.Pointer[func(write bool, n int) (int, bool)]

func streamTestHook(write bool, n int) (int, bool) {
	if hook := streamTestIO.Load(); hook != nil {
		return (*hook)(write, n)
	}
	return n, false
}

func headerU32(hdr []byte, offset int) uint32 {
	return binary.NativeEndian.Uint32(hdr[offset : offset+4])
}

// streamBodyLen returns the bytes that follow a received 32-byte header:
// min(payload_len, packetSize - 32) after an outer header, chunk_payload_len
// after a continuation header, none after any other magic. packetSize 0
// (handshake) caps an outer header at bufLen. Bytes beyond bufLen are not
// counted, as a SEQPACKET recv into a short buffer drops them; the caller's
// length checks then fail.
func streamBodyLen(hdr []byte, bufLen int, packetSize uint32) int {
	// Bounded as uint32 first: a header field above MaxInt32 must not turn
	// negative as an int on 32-bit systems.
	room := uint32(bufLen - protocol.HeaderSize) // #nosec G115 -- callers pass buffers of at least HeaderSize bytes, far below 4 GiB.
	declared := uint32(0)
	switch headerU32(hdr, 0) {
	case protocol.MagicMsg:
		limit := room
		if packetSize > protocol.HeaderSize {
			limit = packetSize - protocol.HeaderSize
		}
		declared = min(headerU32(hdr, 16), limit)
	case protocol.MagicChunk:
		declared = headerU32(hdr, 28)
	}
	return int(min(declared, room))
}

// streamReadExact fills dst, waiting before each read when wait is set (nil
// blocks). EOF before the first byte is a disconnect (ErrRecv); EOF inside a
// packet (any byte read, or midPacket) is a truncated packet (ErrProtocol).
func streamReadExact(fd int, dst []byte, wait *receiveWait, midPacket bool) error {
	var got int
	return streamReadExactCounted(fd, dst, wait, midPacket, &got)
}

// streamReadExactCounted is streamReadExact that adds to *got the bytes it
// reads, also on failure.
func streamReadExactCounted(fd int, dst []byte, wait *receiveWait, midPacket bool, got *int) error {
	start := *got
	for done := 0; done < len(dst); done = *got - start {
		if wait != nil {
			if err := pollReadableForReceive(fd, *wait); err != nil {
				return err
			}
		}
		limit, eintr := streamTestHook(false, len(dst)-done)
		n := 0
		var err error = syscall.EINTR
		if !eintr {
			n, _, _, _, err = syscall.Recvmsg(fd, dst[done:done+limit], nil, 0)
		}
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil {
			return wrapErr(ErrRecv, err.Error())
		}
		if n == 0 {
			if midPacket || done > 0 {
				return wrapErr(ErrProtocol, "truncated packet")
			}
			return wrapErr(ErrRecv, "peer disconnected")
		}
		*got += n
	}
	return nil
}

// streamRecvPacket reads one packet (header, then its body).
func streamRecvPacket(fd int, buf []byte, packetSize uint32, wait *receiveWait) (int, error) {
	if len(buf) < protocol.HeaderSize {
		return 0, wrapErr(ErrBadParam, "packet buffer too small")
	}
	if err := streamReadExact(fd, buf[:protocol.HeaderSize], wait, false); err != nil {
		return 0, err
	}
	body := streamBodyLen(buf[:protocol.HeaderSize], len(buf), packetSize)
	end := protocol.HeaderSize + body
	if err := streamReadExact(fd, buf[protocol.HeaderSize:end], wait, true); err != nil {
		return 0, err
	}
	return end, nil
}

// streamReadExactPending is streamReadExact that takes the session's
// pending bytes first; a nil pending reads only the socket.
// It sets *consumed when it takes any byte, also on failure.
func streamReadExactPending(fd int, pending *pendingQueue, dst []byte, wait *receiveWait, midPacket bool, consumed *bool) error {
	got := 0
	if pending != nil {
		got = pending.take(dst)
	}
	if got > 0 {
		*consumed = true
	}
	if got == len(dst) {
		return nil
	}
	read := 0
	err := streamReadExactCounted(fd, dst[got:], wait, midPacket || got > 0, &read)
	if read > 0 {
		*consumed = true
	}
	return err
}

// streamRecvSessionPacket reads one packet of a session's message, from its
// pending bytes first (nil reads only the socket). The first
// packet's header is checked before its body is read: one that is not an
// outer header, or declares more than this direction's limits, comes back
// alone so the framing rejects it without waiting for bytes a stream peer
// may never send. A truncated continuation is a chunk error, as on
// SEQPACKET. *consumed is set once any byte of the message has been taken.
func streamRecvSessionPacket(fd int, pending *pendingQueue, buf []byte, first bool, packetSize, maxPayload, maxItems uint32, wait *receiveWait, consumed *bool) (int, error) {
	if len(buf) < protocol.HeaderSize {
		return 0, wrapErr(ErrProtocol, "packet too short for header")
	}
	n, err := func() (int, error) {
		if err := streamReadExactPending(fd, pending, buf[:protocol.HeaderSize], wait, false, consumed); err != nil {
			return 0, err
		}
		hdr := buf[:protocol.HeaderSize]
		if first {
			h, err := protocol.DecodeHeader(hdr)
			if err != nil || h.PayloadLen > maxPayload || h.ItemCount > maxItems {
				return protocol.HeaderSize, nil
			}
		}
		end := protocol.HeaderSize + streamBodyLen(hdr, len(buf), packetSize)
		if err := streamReadExactPending(fd, pending, buf[protocol.HeaderSize:end], wait, true, consumed); err != nil {
			return 0, err
		}
		return end, nil
	}()
	if err != nil && !first && errors.Is(err, ErrProtocol) {
		return 0, wrapErr(ErrChunk, err.Error())
	}
	return n, err
}

// streamSendAll sends hdr then payload completely, resuming after partial
// writes. A stream needs no single-call send: the receiver delimits packets
// by their headers.
func streamSendAll(fd int, hdr []byte, payload []byte) error {
	parts := [2][]byte{hdr, payload}
	first := 0
	for first < len(parts) {
		if len(parts[first]) == 0 {
			first++
			continue
		}
		var iov [2]syscall.Iovec
		msg := syscall.Msghdr{Iov: &iov[0]}
		count, total := 0, 0
		for _, part := range parts[first:] {
			if len(part) > 0 {
				iov[count].Base = unsafe.SliceData(part) // #nosec G103 -- sendmsg iovec needs the backing byte-slice pointer.
				iov[count].SetLen(len(part))
				count++
				total += len(part)
			}
		}
		// Tests may cut the write short (to limit bytes) or fail it.
		limit, eintr := streamTestHook(true, total)
		if limit < len(parts[first]) {
			iov[0].SetLen(limit)
			count = 1
		} else if limit < total {
			iov[1].SetLen(limit - len(parts[first]))
		}
		// Msghdr.Iovlen differs in type across platforms.
		msg.Iovlen = 1
		if count == 2 {
			msg.Iovlen = 2
		}

		// A stream write may move part of the data; EINTR means none moved.
		var n uintptr
		errno := syscall.EINTR
		if !eintr {
			n, errno = rawSendmsg(fd, &msg, sendFlags)
		}
		if errno == syscall.EINTR {
			continue
		}
		if errno != 0 {
			return wrapErr(ErrSend, errno.Error())
		}
		if n == 0 {
			return wrapErr(ErrSend, "stream write moved no data")
		}

		left := int(n)
		for first < len(parts) && left >= len(parts[first]) {
			left -= len(parts[first])
			first++
		}
		if first < len(parts) {
			parts[first] = parts[first][left:]
		}
	}
	return nil
}
