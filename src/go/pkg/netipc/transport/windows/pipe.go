//go:build windows

// Package windows implements the L1 Windows Named Pipe transport.
//
// Connection lifecycle, handshake with profile/limit negotiation,
// and send/receive with transparent chunking over Win32 Named Pipes
// in message mode. Wire-compatible with the C and Rust implementations.
//
// Pure Go — no cgo. Works with CGO_ENABLED=0.
package windows

import (
	"errors"
	"fmt"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// ---------------------------------------------------------------------------
//  Constants
// ---------------------------------------------------------------------------

const (
	defaultBatchItems   uint32 = 1
	defaultPacketSize   uint32 = 65536
	defaultPipeBufSize  uint32 = 65536
	helloPayloadSize           = 44
	helloAckPayloadSize        = 48
	maxPipeNameChars           = 256

	// FNV-1a 64-bit constants
	fnv1aOffsetBasis uint64 = 0xcbf29ce484222325
	fnv1aPrime       uint64 = 0x00000100000001B3
)

// Win32 constants
const (
	_PIPE_ACCESS_DUPLEX            = 0x00000003
	_FILE_FLAG_FIRST_PIPE_INSTANCE = 0x00080000
	_PIPE_TYPE_MESSAGE             = 0x00000004
	_PIPE_READMODE_MESSAGE         = 0x00000002
	_PIPE_WAIT                     = 0x00000000
	_PIPE_UNLIMITED_INSTANCES      = 255
	_GENERIC_READ                  = 0x80000000
	_GENERIC_WRITE                 = 0x40000000
	_OPEN_EXISTING                 = 3

	_ERROR_PIPE_CONNECTED     = 535
	_ERROR_BROKEN_PIPE        = 109
	_ERROR_NO_DATA            = 232
	_ERROR_PIPE_NOT_CONNECTED = 233
	_ERROR_ACCESS_DENIED      = 5
	_ERROR_PIPE_BUSY          = 231
)

// ---------------------------------------------------------------------------
//  Errors
// ---------------------------------------------------------------------------

var (
	ErrPipeName       = errors.New("pipe name derivation failed")
	ErrCreatePipe     = errors.New("CreateNamedPipe failed")
	ErrConnect        = errors.New("connect failed")
	ErrAccept         = errors.New("accept failed")
	ErrSend           = errors.New("send failed")
	ErrRecv           = errors.New("recv failed or peer disconnected")
	ErrHandshake      = errors.New("handshake protocol error")
	ErrAuthFailed     = errors.New("authentication token rejected")
	ErrNoProfile      = errors.New("no common transport profile")
	ErrIncompatible   = errors.New("protocol or layout version mismatch")
	ErrProtocol       = errors.New("wire protocol violation")
	ErrAddrInUse      = errors.New("pipe name already in use by live server")
	ErrChunk          = errors.New("chunk header mismatch")
	ErrLimitExceeded  = errors.New("negotiated limit exceeded")
	ErrBadParam       = errors.New("invalid argument")
	ErrDuplicateMsgID = errors.New("duplicate message_id")
	ErrUnknownMsgID   = errors.New("unknown response message_id")
	ErrDisconnected   = errors.New("peer disconnected")
	ErrTimeout        = errors.New("receive deadline expired")
	ErrAborted        = errors.New("receive aborted")
)

func wrapErr(sentinel error, detail string) error {
	return fmt.Errorf("%w: %s", sentinel, detail)
}

// ---------------------------------------------------------------------------
//  Win32 syscall imports (pure Go, no cgo)
// ---------------------------------------------------------------------------

var (
	modkernel32 = syscall.NewLazyDLL("kernel32.dll")

	procCreateNamedPipeW        = modkernel32.NewProc("CreateNamedPipeW")
	procConnectNamedPipe        = modkernel32.NewProc("ConnectNamedPipe")
	procDisconnectNamedPipe     = modkernel32.NewProc("DisconnectNamedPipe")
	procFlushFileBuffers        = modkernel32.NewProc("FlushFileBuffers")
	procPeekNamedPipe           = modkernel32.NewProc("PeekNamedPipe")
	procSetNamedPipeHandleState = modkernel32.NewProc("SetNamedPipeHandleState")
	procSwitchToThread          = modkernel32.NewProc("SwitchToThread")
)

func createNamedPipe(name *uint16, openMode, pipeMode, maxInstances, outBufSize, inBufSize, defaultTimeout uint32) (syscall.Handle, error) {
	r, _, err := procCreateNamedPipeW.Call(
		uintptr(unsafe.Pointer(name)),
		uintptr(openMode),
		uintptr(pipeMode),
		uintptr(maxInstances),
		uintptr(outBufSize),
		uintptr(inBufSize),
		uintptr(defaultTimeout),
		0, // NULL security attributes
	)
	handle := syscall.Handle(r)
	if handle == syscall.InvalidHandle {
		return handle, err
	}
	return handle, nil
}

func connectNamedPipe(handle syscall.Handle) error {
	r, _, err := procConnectNamedPipe.Call(uintptr(handle), 0)
	if r == 0 {
		return err
	}
	return nil
}

func disconnectNamedPipe(handle syscall.Handle) {
	procDisconnectNamedPipe.Call(uintptr(handle))
}

func flushFileBuffers(handle syscall.Handle) {
	procFlushFileBuffers.Call(uintptr(handle))
}

func peekNamedPipeAvailable(handle syscall.Handle) (uint32, error) {
	var available uint32
	r, _, err := procPeekNamedPipe.Call(
		uintptr(handle),
		0,
		0,
		0,
		uintptr(unsafe.Pointer(&available)),
		0,
	)
	if r == 0 {
		return 0, err
	}
	return available, nil
}

func setNamedPipeHandleState(handle syscall.Handle, mode *uint32) error {
	r, _, err := procSetNamedPipeHandleState.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(mode)),
		0, 0,
	)
	if r == 0 {
		return err
	}
	return nil
}

// ---------------------------------------------------------------------------
//  FNV-1a 64-bit hash
// ---------------------------------------------------------------------------

// FNV1a64 computes the FNV-1a 64-bit hash of data.
func FNV1a64(data []byte) uint64 {
	hash := fnv1aOffsetBasis
	for _, b := range data {
		hash ^= uint64(b)
		hash *= fnv1aPrime
	}
	return hash
}

// ---------------------------------------------------------------------------
//  Service name validation
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
//  Pipe name derivation
// ---------------------------------------------------------------------------

// BuildPipeName constructs the Named Pipe path from run_dir and service_name.
// Returns the pipe name as a NUL-terminated UTF-16 slice.
func BuildPipeName(runDir, serviceName string) ([]uint16, error) {
	if err := validateServiceName(serviceName); err != nil {
		return nil, err
	}

	hash := FNV1a64([]byte(runDir))
	narrow := fmt.Sprintf(`\\.\pipe\netipc-%016x-%s`, hash, serviceName)

	if len(narrow) >= maxPipeNameChars {
		return nil, wrapErr(ErrPipeName, "pipe name too long")
	}

	return utf16.Encode(append([]rune(narrow), 0)), nil
}

// ---------------------------------------------------------------------------
//  Internal helpers
// ---------------------------------------------------------------------------

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

func maxU32(a, b uint32) uint32 {
	if a > b {
		return a
	}
	return b
}

func pipeBufferSize(packetSize uint32) uint32 {
	// The protocol packet size controls logical framing and chunk size. The
	// underlying pipe quota must stay large enough for full-duplex pipelining
	// even when tests force a tiny protocol packet size.
	return maxU32(applyDefault(packetSize, defaultPipeBufSize), defaultPipeBufSize)
}

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

func isDisconnectError(err error) bool {
	errno, ok := err.(syscall.Errno)
	if !ok {
		return false
	}
	return errno == _ERROR_BROKEN_PIPE ||
		errno == _ERROR_NO_DATA ||
		errno == _ERROR_PIPE_NOT_CONNECTED
}

// ---------------------------------------------------------------------------
//  Low-level I/O
// ---------------------------------------------------------------------------

func rawWrite(handle syscall.Handle, data []byte) error {
	var written uint32
	err := syscall.WriteFile(handle, data, &written, nil)
	if err != nil {
		if isDisconnectError(err) {
			return ErrDisconnected
		}
		return wrapErr(ErrSend, err.Error())
	}
	if written != uint32(len(data)) {
		return wrapErr(ErrSend, fmt.Sprintf("short write: %d/%d", written, len(data)))
	}
	return nil
}

func rawSendMsg(handle syscall.Handle, msg []byte) error {
	return rawWrite(handle, msg)
}

func rawRecv(handle syscall.Handle, buf []byte) (int, error) {
	var read uint32
	err := syscall.ReadFile(handle, buf, &read, nil)
	if err != nil {
		// ERROR_MORE_DATA (234): message mode pipe message is larger
		// than the buffer. The data read so far is valid; the
		// remaining data can be read with another ReadFile call.
		// For our protocol this should not happen if the buffer is
		// sized correctly, but treat it as a successful partial read
		// rather than a fatal error.
		if err == syscall.Errno(234) {
			if read > 0 {
				return int(read), nil
			}
		}
		if isDisconnectError(err) {
			return 0, ErrDisconnected
		}
		return 0, wrapErr(ErrRecv, err.Error())
	}
	if read == 0 {
		return 0, ErrDisconnected
	}
	return int(read), nil
}

func ensurePipeScratchBuf(buf *[]byte, needed int) []byte {
	if len(*buf) < needed {
		*buf = make([]byte, needed)
	}
	return (*buf)[:needed]
}
