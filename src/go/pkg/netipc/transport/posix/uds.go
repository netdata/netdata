//go:build unix

// Package posix implements the L1 POSIX UDS SEQPACKET transport.
//
// Connection lifecycle, handshake with profile/limit negotiation,
// and send/receive with transparent chunking over AF_UNIX SEQPACKET sockets.
// Wire-compatible with the C and Rust implementations.
//
// Pure Go — no cgo. Works with CGO_ENABLED=0.
package posix

import (
	"errors"
	"fmt"
	"path/filepath"
	"syscall"
	"unsafe"
)

// ---------------------------------------------------------------------------
//  Constants
// ---------------------------------------------------------------------------

const (
	defaultBacklog                   = 16
	defaultBatchItems                = 1
	defaultPacketSizeFallback uint32 = 65536
	helloPayloadSize                 = 44
	helloAckPayloadSize              = 48

	// sun_path max — 108 on Linux, 104 on macOS/FreeBSD.
	// We use a conservative limit.
	maxSunPath = 104
)

// ---------------------------------------------------------------------------
//  Errors
// ---------------------------------------------------------------------------

var (
	ErrPathTooLong    = errors.New("socket path exceeds sun_path limit")
	ErrSocket         = errors.New("socket syscall failed")
	ErrConnect        = errors.New("connect failed")
	ErrAccept         = errors.New("accept failed")
	ErrSend           = errors.New("send failed")
	ErrRecv           = errors.New("recv failed or peer disconnected")
	ErrHandshake      = errors.New("handshake protocol error")
	ErrAuthFailed     = errors.New("authentication token rejected")
	ErrNoProfile      = errors.New("no common transport profile")
	ErrIncompatible   = errors.New("protocol or layout version mismatch")
	ErrProtocol       = errors.New("wire protocol violation")
	ErrAddrInUse      = errors.New("address already in use by live server")
	ErrChunk          = errors.New("chunk header mismatch")
	ErrLimitExceeded  = errors.New("negotiated limit exceeded")
	ErrBadParam       = errors.New("invalid argument")
	ErrDuplicateMsgID = errors.New("duplicate message_id")
	ErrUnknownMsgID   = errors.New("unknown response message_id")
)

// wrapErr creates a descriptive error wrapping a sentinel.
func wrapErr(sentinel error, detail string) error {
	return fmt.Errorf("%w: %s", sentinel, detail)
}

// ---------------------------------------------------------------------------
//  Internal helpers
// ---------------------------------------------------------------------------

// validateServiceName checks that name contains only [a-zA-Z0-9._-],
// is non-empty, and is not "." or "..".
func validateServiceName(name string) error {
	if name == "" {
		return wrapErr(ErrBadParam, "empty service name")
	}
	if name == "." || name == ".." {
		return wrapErr(ErrBadParam, "service name cannot be '.' or '..'")
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-' {
			continue
		}
		return wrapErr(ErrBadParam, fmt.Sprintf("service name contains invalid character: %q", c))
	}
	return nil
}

// buildSocketPath constructs {runDir}/{serviceName}.sock and validates length.
func buildSocketPath(runDir, serviceName string) (string, error) {
	if err := validateServiceName(serviceName); err != nil {
		return "", err
	}
	path := filepath.Join(runDir, serviceName+".sock")
	// sun_path limit check. On Linux it's 108, macOS/FreeBSD 104.
	// We use the smaller value for portability.
	if len(path) >= maxSunPath {
		return "", ErrPathTooLong
	}
	return path, nil
}

// detectPacketSize reads SO_SNDBUF from the socket.
func detectPacketSize(fd int) uint32 {
	val, err := syscall.GetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_SNDBUF)
	if err != nil || val <= 0 {
		return defaultPacketSizeFallback
	}
	return uint32(val)
}

// highestBit returns the highest set bit in a bitmask (0 if empty).
func highestBit(mask uint32) uint32 {
	if mask == 0 {
		return 0
	}
	bit := uint32(1) << 31
	for bit&mask == 0 {
		bit >>= 1
	}
	return bit
}

func applyDefault(val, def uint32) uint32 {
	if val == 0 {
		return def
	}
	return val
}

func minU32(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}

// ---------------------------------------------------------------------------
//  Low-level I/O
// ---------------------------------------------------------------------------

// rawSendIov sends header + payload as one SEQPACKET message using sendmsg.
func rawSendIov(fd int, hdr []byte, payload []byte) error {
	var iov [2]syscall.Iovec
	iov[0].Base = unsafe.SliceData(hdr) // #nosec G103 -- sendmsg iovec needs the backing byte-slice pointer.
	iov[0].SetLen(len(hdr))

	iovlen := uint64(1)
	if len(payload) > 0 {
		iov[1].Base = unsafe.SliceData(payload) // #nosec G103 -- sendmsg iovec needs the backing byte-slice pointer.
		iov[1].SetLen(len(payload))
		iovlen = 2
	}

	msg := syscall.Msghdr{
		Iov:    &iov[0],
		Iovlen: iovlen,
	}

	n, _, errno := syscall.Syscall(
		syscall.SYS_SENDMSG,
		uintptr(fd),
		uintptr(unsafe.Pointer(&msg)), // #nosec G103 -- raw sendmsg syscall requires a Msghdr pointer.
		uintptr(syscall.MSG_NOSIGNAL),
	)
	if errno != 0 {
		return wrapErr(ErrSend, errno.Error())
	}

	expected := len(hdr) + len(payload)
	if int(n) != expected {
		return wrapErr(ErrSend, fmt.Sprintf("short write: %d/%d", n, expected))
	}
	return nil
}

func ensureScratchBuf(buf *[]byte, needed int) []byte {
	if len(*buf) < needed {
		*buf = make([]byte, needed)
	}
	return (*buf)[:needed]
}

// rawRecv receives one SEQPACKET message. Returns bytes received.
func rawRecv(fd int, buf []byte) (int, error) {
	n, _, _, _, err := syscall.Recvmsg(fd, buf, nil, 0)
	if err != nil {
		return 0, wrapErr(ErrRecv, err.Error())
	}
	if n == 0 {
		return 0, wrapErr(ErrRecv, "peer disconnected")
	}
	return n, nil
}
