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
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
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
)

func wrapErr(sentinel error, detail string) error {
	return fmt.Errorf("%w: %s", sentinel, detail)
}

func headerVersionIncompatible(buf []byte, expectedCode uint16) bool {
	if len(buf) < protocol.HeaderSize {
		return false
	}

	return binary.NativeEndian.Uint32(buf[0:4]) == protocol.MagicMsg &&
		binary.NativeEndian.Uint16(buf[4:6]) != protocol.Version &&
		binary.NativeEndian.Uint16(buf[6:8]) == protocol.HeaderLen &&
		binary.NativeEndian.Uint16(buf[8:10]) == protocol.KindControl &&
		binary.NativeEndian.Uint16(buf[12:14]) == expectedCode
}

func helloLayoutIncompatible(buf []byte) bool {
	return len(buf) >= 2 && binary.NativeEndian.Uint16(buf[0:2]) != 1
}

func helloAckLayoutIncompatible(buf []byte) bool {
	return len(buf) >= 2 && binary.NativeEndian.Uint16(buf[0:2]) != 1
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
//  Role
// ---------------------------------------------------------------------------

// Role distinguishes client vs server sessions.
type Role int

const (
	RoleClient Role = 1
	RoleServer Role = 2
)

// ---------------------------------------------------------------------------
//  Configuration
// ---------------------------------------------------------------------------

// ClientConfig configures a client connection.
type ClientConfig struct {
	SupportedProfiles       uint32
	PreferredProfiles       uint32
	MaxRequestPayloadBytes  uint32 // 0 = use default (1024)
	MaxRequestBatchItems    uint32 // 0 = use default (1)
	MaxResponsePayloadBytes uint32
	MaxResponseBatchItems   uint32
	AuthToken               uint64
	PacketSize              uint32 // 0 = use default (65536)
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
	PacketSize              uint32 // 0 = use default (65536)
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

// ---------------------------------------------------------------------------
//  Session
// ---------------------------------------------------------------------------

// Session is a connected Named Pipe session (client or server side).
type Session struct {
	handle syscall.Handle
	role   Role

	// Negotiated limits
	MaxRequestPayloadBytes  uint32
	MaxRequestBatchItems    uint32
	MaxResponsePayloadBytes uint32
	MaxResponseBatchItems   uint32
	PacketSize              uint32
	SelectedProfile         uint32
	SessionID               uint64

	// Internal receive buffer for chunked reassembly
	recvBuf []byte

	// Reusable packet scratch buffers for send/receive chunk assembly.
	sendBuf []byte
	pktBuf  []byte

	// In-flight message_id set (client-side only)
	inflightIDs map[uint64]struct{}
}

func (s *Session) failAllInflight() {
	if s.role != RoleClient || len(s.inflightIDs) == 0 {
		return
	}
	clear(s.inflightIDs)
}

// Handle returns the raw HANDLE for WaitForSingleObject integration.
func (s *Session) Handle() syscall.Handle {
	return s.handle
}

// Role returns the session role.
func (s *Session) GetRole() Role {
	return s.role
}

// WaitReadable waits until bytes are available to read or the timeout expires.
func (s *Session) WaitReadable(timeoutMs uint32) (bool, error) {
	if s.handle == syscall.InvalidHandle {
		return false, wrapErr(ErrBadParam, "session closed")
	}

	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	yielded := false
	for {
		available, err := peekNamedPipeAvailable(s.handle)
		if err != nil {
			if isDisconnectError(err) {
				s.failAllInflight()
				return false, ErrDisconnected
			}
			return false, wrapErr(ErrRecv, err.Error())
		}
		if available > 0 {
			return true, nil
		}
		if !time.Now().Before(deadline) {
			return false, nil
		}
		if !yielded {
			yielded = true
			for i := 0; i < 256; i++ {
				procSwitchToThread.Call()
				available, err = peekNamedPipeAvailable(s.handle)
				if err != nil {
					if isDisconnectError(err) {
						s.failAllInflight()
						return false, ErrDisconnected
					}
					return false, wrapErr(ErrRecv, err.Error())
				}
				if available > 0 {
					return true, nil
				}
				if !time.Now().Before(deadline) {
					return false, nil
				}
			}
			continue
		}
		time.Sleep(time.Millisecond)
	}
}

// Close closes the session and releases resources.
func (s *Session) Close() {
	if s.handle != syscall.InvalidHandle {
		if s.role == RoleServer {
			// Flush before server-side disconnect so the client can
			// consume any final response bytes already written.
			flushFileBuffers(s.handle)
			disconnectNamedPipe(s.handle)
		}
		syscall.CloseHandle(s.handle)
		s.handle = syscall.InvalidHandle
	}
	s.recvBuf = nil
	s.sendBuf = nil
	s.pktBuf = nil
	s.failAllInflight()
}

// Connect establishes a session to a server pipe derived from runDir + serviceName.
func Connect(runDir, serviceName string, config *ClientConfig) (*Session, error) {
	pipeName, err := BuildPipeName(runDir, serviceName)
	if err != nil {
		return nil, err
	}

	handle, err := syscall.CreateFile(
		&pipeName[0],
		_GENERIC_READ|_GENERIC_WRITE,
		0,
		nil,
		_OPEN_EXISTING,
		0,
		0,
	)
	if err != nil {
		return nil, wrapErr(ErrConnect, err.Error())
	}

	// Set read mode to message mode
	mode := uint32(_PIPE_READMODE_MESSAGE)
	if err := setNamedPipeHandleState(handle, &mode); err != nil {
		syscall.CloseHandle(handle)
		return nil, wrapErr(ErrConnect, "SetNamedPipeHandleState: "+err.Error())
	}

	session, herr := clientHandshake(handle, config)
	if herr != nil {
		syscall.CloseHandle(handle)
		return nil, herr
	}
	return session, nil
}

// Send sends one logical message. Fills magic/version/header_len/payload_len.
func (s *Session) Send(hdr *protocol.Header, payload []byte) error {
	if s.handle == syscall.InvalidHandle {
		return wrapErr(ErrBadParam, "session closed")
	}

	// Client-side: track in-flight message_ids
	if s.role == RoleClient && hdr.Kind == protocol.KindRequest {
		if s.inflightIDs == nil {
			s.inflightIDs = make(map[uint64]struct{})
		}
		if _, exists := s.inflightIDs[hdr.MessageID]; exists {
			return wrapErr(ErrDuplicateMsgID, fmt.Sprintf("message_id %d", hdr.MessageID))
		}
		s.inflightIDs[hdr.MessageID] = struct{}{}
	}

	// Fill envelope
	hdr.Magic = protocol.MagicMsg
	hdr.Version = protocol.Version
	hdr.HeaderLen = protocol.HeaderLen
	hdr.PayloadLen = uint32(len(payload))

	tracked := s.role == RoleClient && hdr.Kind == protocol.KindRequest

	sendErr := s.sendInner(hdr, payload)

	if sendErr != nil && tracked {
		if errors.Is(sendErr, ErrDisconnected) {
			s.failAllInflight()
		} else {
			delete(s.inflightIDs, hdr.MessageID)
		}
	}

	return sendErr
}

func (s *Session) sendInner(hdr *protocol.Header, payload []byte) error {
	totalMsg := protocol.HeaderSize + len(payload)

	// Single packet?
	if totalMsg <= int(s.PacketSize) {
		msg := ensurePipeScratchBuf(&s.sendBuf, totalMsg)
		hdr.Encode(msg[:protocol.HeaderSize])
		copy(msg[protocol.HeaderSize:], payload)
		return rawSendMsg(s.handle, msg[:totalMsg])
	}

	// Chunked send
	chunkPayloadBudget := int(s.PacketSize) - protocol.HeaderSize
	if chunkPayloadBudget <= 0 {
		return wrapErr(ErrBadParam, "packet_size too small")
	}

	firstChunkPayload := len(payload)
	if firstChunkPayload > chunkPayloadBudget {
		firstChunkPayload = chunkPayloadBudget
	}

	remainingAfterFirst := len(payload) - firstChunkPayload
	continuationChunks := uint32(0)
	if remainingAfterFirst > 0 {
		continuationChunks = uint32((remainingAfterFirst + chunkPayloadBudget - 1) / chunkPayloadBudget)
	}
	chunkCount := 1 + continuationChunks

	// First chunk
	pktBuf := ensurePipeScratchBuf(&s.sendBuf, int(s.PacketSize))
	hdr.Encode(pktBuf[:protocol.HeaderSize])
	copy(pktBuf[protocol.HeaderSize:], payload[:firstChunkPayload])
	if err := rawSendMsg(s.handle, pktBuf[:protocol.HeaderSize+firstChunkPayload]); err != nil {
		return err
	}

	// Continuation chunks
	offset := firstChunkPayload
	for ci := uint32(1); ci < chunkCount; ci++ {
		remaining := len(payload) - offset
		thisChunk := remaining
		if thisChunk > chunkPayloadBudget {
			thisChunk = chunkPayloadBudget
		}

		chk := protocol.ChunkHeader{
			Magic:           protocol.MagicChunk,
			Version:         protocol.Version,
			Flags:           0,
			MessageID:       hdr.MessageID,
			TotalMessageLen: uint32(totalMsg),
			ChunkIndex:      ci,
			ChunkCount:      chunkCount,
			ChunkPayloadLen: uint32(thisChunk),
		}

		chk.Encode(pktBuf[:protocol.HeaderSize])
		copy(pktBuf[protocol.HeaderSize:], payload[offset:offset+thisChunk])
		if err := rawSendMsg(s.handle, pktBuf[:protocol.HeaderSize+thisChunk]); err != nil {
			return err
		}

		offset += thisChunk
	}

	return nil
}

// Receive reads one logical message. buf is a scratch buffer.
func (s *Session) Receive(buf []byte) (protocol.Header, []byte, error) {
	if s.handle == syscall.InvalidHandle {
		return protocol.Header{}, nil, wrapErr(ErrBadParam, "session closed")
	}

	n, err := rawRecv(s.handle, buf)
	if err != nil {
		if errors.Is(err, ErrDisconnected) {
			s.failAllInflight()
		}
		return protocol.Header{}, nil, err
	}

	if n < protocol.HeaderSize {
		return protocol.Header{}, nil, wrapErr(ErrProtocol, "packet too short for header")
	}

	hdr, err := protocol.DecodeHeader(buf[:n])
	if err != nil {
		return protocol.Header{}, nil, wrapErr(ErrProtocol, "header decode: "+err.Error())
	}

	// Validate payload_len
	var maxPayload uint32
	if s.role == RoleServer {
		maxPayload = s.MaxRequestPayloadBytes
	} else {
		maxPayload = s.MaxResponsePayloadBytes
	}
	if hdr.PayloadLen > maxPayload {
		return protocol.Header{}, nil, wrapErr(ErrLimitExceeded,
			fmt.Sprintf("payload_len %d exceeds negotiated max %d", hdr.PayloadLen, maxPayload))
	}

	// Validate item_count
	var maxBatch uint32
	if s.role == RoleServer {
		maxBatch = s.MaxRequestBatchItems
	} else {
		maxBatch = s.MaxResponseBatchItems
	}
	if hdr.ItemCount > maxBatch {
		return protocol.Header{}, nil, wrapErr(ErrLimitExceeded,
			fmt.Sprintf("item_count %d exceeds negotiated max %d", hdr.ItemCount, maxBatch))
	}

	// Client-side: validate response message_id
	if s.role == RoleClient && hdr.Kind == protocol.KindResponse {
		if s.inflightIDs == nil {
			return protocol.Header{}, nil, wrapErr(ErrUnknownMsgID,
				fmt.Sprintf("message_id %d", hdr.MessageID))
		}
		if _, exists := s.inflightIDs[hdr.MessageID]; !exists {
			return protocol.Header{}, nil, wrapErr(ErrUnknownMsgID,
				fmt.Sprintf("message_id %d", hdr.MessageID))
		}
		delete(s.inflightIDs, hdr.MessageID)
	}

	totalMsg := protocol.HeaderSize + int(hdr.PayloadLen)

	// Non-chunked
	if n >= totalMsg {
		payload := buf[protocol.HeaderSize : protocol.HeaderSize+int(hdr.PayloadLen)]

		// Validate batch directory
		if hdr.Flags&protocol.FlagBatch != 0 && hdr.ItemCount > 1 {
			dirBytes := int(hdr.ItemCount) * 8
			dirAligned := protocol.Align8(dirBytes)
			if len(payload) < dirAligned {
				return protocol.Header{}, nil, wrapErr(ErrProtocol, "batch dir exceeds payload")
			}
			packedAreaLen := uint32(len(payload) - dirAligned)
			if err := protocol.BatchDirValidate(payload[:dirBytes], hdr.ItemCount, packedAreaLen); err != nil {
				return protocol.Header{}, nil, wrapErr(ErrProtocol, "batch dir: "+err.Error())
			}
		}

		return hdr, payload, nil
	}

	// Chunked
	firstPayloadBytes := n - protocol.HeaderSize
	needed := int(hdr.PayloadLen)
	if len(s.recvBuf) < needed {
		s.recvBuf = make([]byte, needed)
	}

	copy(s.recvBuf[:firstPayloadBytes], buf[protocol.HeaderSize:protocol.HeaderSize+firstPayloadBytes])

	assembled := firstPayloadBytes
	chunkPayloadBudget := int(s.PacketSize) - protocol.HeaderSize

	remainingAfterFirst := int(hdr.PayloadLen) - firstPayloadBytes
	expectedContinuations := uint32(0)
	if remainingAfterFirst > 0 && chunkPayloadBudget > 0 {
		expectedContinuations = uint32((remainingAfterFirst + chunkPayloadBudget - 1) / chunkPayloadBudget)
	}
	expectedChunkCount := 1 + expectedContinuations

	pktBuf := ensurePipeScratchBuf(&s.pktBuf, int(s.PacketSize))

	ci := uint32(1)
	for assembled < int(hdr.PayloadLen) {
		cn, err := rawRecv(s.handle, pktBuf)
		if err != nil {
			if errors.Is(err, ErrDisconnected) {
				s.failAllInflight()
			}
			return protocol.Header{}, nil, wrapErr(ErrRecv, "continuation recv: "+err.Error())
		}

		if cn < protocol.HeaderSize {
			return protocol.Header{}, nil, wrapErr(ErrChunk, "continuation too short")
		}

		chk, err := protocol.DecodeChunkHeader(pktBuf[:cn])
		if err != nil {
			return protocol.Header{}, nil, wrapErr(ErrChunk, "chunk header: "+err.Error())
		}

		if chk.MessageID != hdr.MessageID {
			return protocol.Header{}, nil, wrapErr(ErrChunk, "message_id mismatch")
		}
		if chk.ChunkIndex != ci {
			return protocol.Header{}, nil, wrapErr(ErrChunk, fmt.Sprintf(
				"chunk_index mismatch: expected %d, got %d", ci, chk.ChunkIndex))
		}
		if chk.ChunkCount != expectedChunkCount {
			return protocol.Header{}, nil, wrapErr(ErrChunk, "chunk_count mismatch")
		}
		if chk.TotalMessageLen != uint32(totalMsg) {
			return protocol.Header{}, nil, wrapErr(ErrChunk, "total_message_len mismatch")
		}

		chunkData := cn - protocol.HeaderSize
		if chunkData != int(chk.ChunkPayloadLen) {
			return protocol.Header{}, nil, wrapErr(ErrChunk, "chunk_payload_len mismatch")
		}
		if assembled+chunkData > int(hdr.PayloadLen) {
			return protocol.Header{}, nil, wrapErr(ErrChunk, "chunk exceeds payload_len")
		}

		copy(s.recvBuf[assembled:assembled+chunkData], pktBuf[protocol.HeaderSize:protocol.HeaderSize+chunkData])
		assembled += chunkData
		ci++
	}

	payload := s.recvBuf[:hdr.PayloadLen]

	// Validate batch directory
	if hdr.Flags&protocol.FlagBatch != 0 && hdr.ItemCount > 1 {
		dirBytes := int(hdr.ItemCount) * 8
		dirAligned := protocol.Align8(dirBytes)
		if len(payload) < dirAligned {
			return protocol.Header{}, nil, wrapErr(ErrProtocol, "batch dir exceeds payload")
		}
		packedAreaLen := uint32(len(payload) - dirAligned)
		if err := protocol.BatchDirValidate(payload[:dirBytes], hdr.ItemCount, packedAreaLen); err != nil {
			return protocol.Header{}, nil, wrapErr(ErrProtocol, "batch dir: "+err.Error())
		}
	}

	return hdr, payload, nil
}

func ensurePipeScratchBuf(buf *[]byte, needed int) []byte {
	if len(*buf) < needed {
		*buf = make([]byte, needed)
	}
	return (*buf)[:needed]
}

// ---------------------------------------------------------------------------
//  Listener
// ---------------------------------------------------------------------------

// Listener is a listening Named Pipe endpoint.
type Listener struct {
	mu            sync.Mutex
	handle        syscall.Handle
	config        ServerConfig
	pipeName      []uint16
	nextSessionID atomic.Uint64
	closing       bool
	accepting     bool
}

// Listen creates a listener on a Named Pipe derived from runDir + serviceName.
func Listen(runDir, serviceName string, config ServerConfig) (*Listener, error) {
	pipeName, err := BuildPipeName(runDir, serviceName)
	if err != nil {
		return nil, err
	}

	bufSize := pipeBufferSize(config.PacketSize)

	// Create first instance with FILE_FLAG_FIRST_PIPE_INSTANCE
	handle, err := createPipeInstance(pipeName, bufSize, true)
	if err != nil {
		return nil, err
	}

	return &Listener{
		handle:   handle,
		config:   config,
		pipeName: pipeName,
	}, nil
}

// Handle returns the raw HANDLE.
func (l *Listener) Handle() syscall.Handle {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.handle
}

// SetPayloadLimits updates the payload limits used for future handshakes.
func (l *Listener) SetPayloadLimits(maxRequestPayloadBytes, maxResponsePayloadBytes uint32) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.config.MaxRequestPayloadBytes = maxRequestPayloadBytes
	l.config.MaxResponsePayloadBytes = maxResponsePayloadBytes
}

// Accept accepts one client connection. Performs the full handshake.
func (l *Listener) Accept() (*Session, error) {
	sessionID := l.nextSessionID.Add(1)
	return l.AcceptWithConfig(sessionID, l.config)
}

// AcceptWithConfig accepts one client connection using a caller-provided
// per-session server config and session ID.
func (l *Listener) AcceptWithConfig(sessionID uint64, config ServerConfig) (*Session, error) {
	l.mu.Lock()
	if l.handle == syscall.InvalidHandle {
		l.mu.Unlock()
		return nil, wrapErr(ErrAccept, "listener closed")
	}
	sessionHandle := l.handle
	l.accepting = true
	l.mu.Unlock()

	err := connectNamedPipe(sessionHandle)
	if err != nil {
		// ERROR_PIPE_CONNECTED is fine — client connected between
		// CreateNamedPipe and ConnectNamedPipe
		if errno, ok := err.(syscall.Errno); !ok || errno != _ERROR_PIPE_CONNECTED {
			l.mu.Lock()
			l.accepting = false
			l.mu.Unlock()
			return nil, wrapErr(ErrAccept, err.Error())
		}
	}

	l.mu.Lock()
	l.accepting = false
	if l.closing {
		if l.handle == sessionHandle {
			l.handle = syscall.InvalidHandle
		}
		l.mu.Unlock()
		disconnectNamedPipe(sessionHandle)
		syscall.CloseHandle(sessionHandle)
		return nil, wrapErr(ErrAccept, "listener closed")
	}

	// Create new pipe instance for next client
	bufSize := pipeBufferSize(l.config.PacketSize)
	next, perr := createPipeInstance(l.pipeName, bufSize, false)
	if perr != nil {
		if l.handle == sessionHandle {
			l.handle = syscall.InvalidHandle
		}
		l.mu.Unlock()
		disconnectNamedPipe(sessionHandle)
		syscall.CloseHandle(sessionHandle)
		return nil, perr
	}
	l.handle = next
	l.mu.Unlock()

	// Handshake
	session, herr := serverHandshake(sessionHandle, &config, sessionID)
	if herr != nil {
		disconnectNamedPipe(sessionHandle)
		syscall.CloseHandle(sessionHandle)
		return nil, herr
	}
	return session, nil
}

// Close closes the listener.
func (l *Listener) Close() {
	l.mu.Lock()
	handle := l.handle
	if handle == syscall.InvalidHandle {
		l.mu.Unlock()
		return
	}
	l.closing = true
	accepting := l.accepting
	if !accepting {
		l.handle = syscall.InvalidHandle
	}
	pipeName := l.pipeName
	l.mu.Unlock()

	if accepting && len(pipeName) > 0 && pipeName[0] != 0 {
		// A loopback connect reliably wakes a blocking ConnectNamedPipe()
		// so Accept() can observe shutdown and close the live listener handle
		// from the owning goroutine.
		wake, err := syscall.CreateFile(
			&pipeName[0],
			_GENERIC_READ|_GENERIC_WRITE,
			0,
			nil,
			_OPEN_EXISTING,
			0,
			0,
		)
		if err == nil && wake != syscall.InvalidHandle && wake != 0 {
			syscall.CloseHandle(wake)
		}
		return
	}

	syscall.CloseHandle(handle)
}

// ---------------------------------------------------------------------------
//  Pipe instance creation
// ---------------------------------------------------------------------------

func createPipeInstance(pipeName []uint16, bufSize uint32, firstInstance bool) (syscall.Handle, error) {
	openMode := uint32(_PIPE_ACCESS_DUPLEX)
	if firstInstance {
		openMode |= _FILE_FLAG_FIRST_PIPE_INSTANCE
	}

	handle, err := createNamedPipe(
		&pipeName[0],
		openMode,
		_PIPE_TYPE_MESSAGE|_PIPE_READMODE_MESSAGE|_PIPE_WAIT,
		_PIPE_UNLIMITED_INSTANCES,
		bufSize,
		bufSize,
		0,
	)
	if err != nil {
		errno, ok := err.(syscall.Errno)
		if ok && (errno == _ERROR_ACCESS_DENIED || errno == _ERROR_PIPE_BUSY) {
			return syscall.InvalidHandle, ErrAddrInUse
		}
		return syscall.InvalidHandle, wrapErr(ErrCreatePipe, err.Error())
	}
	return handle, nil
}

// ---------------------------------------------------------------------------
//  Client handshake
// ---------------------------------------------------------------------------

func clientHandshake(handle syscall.Handle, config *ClientConfig) (*Session, error) {
	pktSize := applyDefault(config.PacketSize, defaultPacketSize)

	supported := config.SupportedProfiles
	if supported == 0 {
		supported = protocol.ProfileBaseline
	}

	hello := protocol.Hello{
		LayoutVersion:           1,
		Flags:                   0,
		SupportedProfiles:       supported,
		PreferredProfiles:       config.PreferredProfiles,
		MaxRequestPayloadBytes:  applyDefault(config.MaxRequestPayloadBytes, protocol.MaxPayloadDefault),
		MaxRequestBatchItems:    applyDefault(config.MaxRequestBatchItems, defaultBatchItems),
		MaxResponsePayloadBytes: applyDefault(config.MaxResponsePayloadBytes, protocol.MaxPayloadDefault),
		MaxResponseBatchItems:   applyDefault(config.MaxResponseBatchItems, defaultBatchItems),
		AuthToken:               config.AuthToken,
		PacketSize:              pktSize,
	}

	var helloBuf [helloPayloadSize]byte
	hello.Encode(helloBuf[:])

	hdr := protocol.Header{
		Magic:           protocol.MagicMsg,
		Version:         protocol.Version,
		HeaderLen:       protocol.HeaderLen,
		Kind:            protocol.KindControl,
		Flags:           0,
		Code:            protocol.CodeHello,
		TransportStatus: protocol.StatusOK,
		PayloadLen:      helloPayloadSize,
		ItemCount:       1,
		MessageID:       0,
	}

	var pkt [protocol.HeaderSize + helloPayloadSize]byte
	hdr.Encode(pkt[:protocol.HeaderSize])
	copy(pkt[protocol.HeaderSize:], helloBuf[:])

	// Send HELLO
	if err := rawWrite(handle, pkt[:]); err != nil {
		return nil, wrapErr(ErrSend, "hello send: "+err.Error())
	}

	// Receive HELLO_ACK
	var ackBuf [128]byte
	an, err := rawRecv(handle, ackBuf[:])
	if err != nil {
		return nil, wrapErr(ErrRecv, "hello_ack recv: "+err.Error())
	}

	ackHdr, err := protocol.DecodeHeader(ackBuf[:an])
	if err != nil {
		if errors.Is(err, protocol.ErrBadVersion) {
			return nil, wrapErr(ErrIncompatible, "ack header version mismatch")
		}
		return nil, wrapErr(ErrProtocol, "ack header: "+err.Error())
	}

	if ackHdr.Kind != protocol.KindControl || ackHdr.Code != protocol.CodeHelloAck {
		return nil, wrapErr(ErrProtocol, "expected HELLO_ACK")
	}

	if ackHdr.TransportStatus == protocol.StatusAuthFailed {
		return nil, ErrAuthFailed
	}
	if ackHdr.TransportStatus == protocol.StatusUnsupported {
		return nil, ErrNoProfile
	}
	if ackHdr.TransportStatus == protocol.StatusIncompatible {
		return nil, ErrIncompatible
	}
	if ackHdr.TransportStatus == protocol.StatusLimitExceeded {
		return nil, ErrLimitExceeded
	}
	if ackHdr.TransportStatus != protocol.StatusOK {
		return nil, wrapErr(ErrHandshake, fmt.Sprintf("transport_status=%d", ackHdr.TransportStatus))
	}

	if an < protocol.HeaderSize+helloAckPayloadSize {
		return nil, wrapErr(ErrProtocol, "ack payload truncated")
	}
	ack, err := protocol.DecodeHelloAck(ackBuf[protocol.HeaderSize:an])
	if err != nil {
		if errors.Is(err, protocol.ErrBadLayout) &&
			helloAckLayoutIncompatible(ackBuf[protocol.HeaderSize:an]) {
			return nil, wrapErr(ErrIncompatible, "ack payload layout version mismatch")
		}
		return nil, wrapErr(ErrProtocol, "ack payload: "+err.Error())
	}

	return &Session{
		handle:                  handle,
		role:                    RoleClient,
		MaxRequestPayloadBytes:  ack.AgreedMaxRequestPayloadBytes,
		MaxRequestBatchItems:    ack.AgreedMaxRequestBatchItems,
		MaxResponsePayloadBytes: ack.AgreedMaxResponsePayloadBytes,
		MaxResponseBatchItems:   ack.AgreedMaxResponseBatchItems,
		PacketSize:              ack.AgreedPacketSize,
		SelectedProfile:         ack.SelectedProfile,
		SessionID:               ack.SessionID,
		inflightIDs:             make(map[uint64]struct{}),
	}, nil
}

// ---------------------------------------------------------------------------
//  Server handshake
// ---------------------------------------------------------------------------

func serverHandshake(handle syscall.Handle, config *ServerConfig, sessionID uint64) (*Session, error) {
	serverPktSize := applyDefault(config.PacketSize, defaultPacketSize)
	sRespPay := applyDefault(config.MaxResponsePayloadBytes, protocol.MaxPayloadDefault)
	sProfiles := config.SupportedProfiles
	if sProfiles == 0 {
		sProfiles = protocol.ProfileBaseline
	}
	sPreferred := config.PreferredProfiles

	// Helper: send rejection
	sendRejection := func(status uint16) {
		ack := protocol.HelloAck{LayoutVersion: 1}
		var ackPayBuf [helloAckPayloadSize]byte
		ack.Encode(ackPayBuf[:])

		ackHdr := protocol.Header{
			Magic:           protocol.MagicMsg,
			Version:         protocol.Version,
			HeaderLen:       protocol.HeaderLen,
			Kind:            protocol.KindControl,
			Code:            protocol.CodeHelloAck,
			TransportStatus: status,
			PayloadLen:      helloAckPayloadSize,
			ItemCount:       1,
		}

		var pkt [protocol.HeaderSize + helloAckPayloadSize]byte
		ackHdr.Encode(pkt[:protocol.HeaderSize])
		copy(pkt[protocol.HeaderSize:], ackPayBuf[:])
		rawWrite(handle, pkt[:]) //nolint:errcheck
	}

	// Receive HELLO
	var buf [128]byte
	n, err := rawRecv(handle, buf[:])
	if err != nil {
		return nil, wrapErr(ErrRecv, "hello recv: "+err.Error())
	}

	hdr, err := protocol.DecodeHeader(buf[:n])
	if err != nil {
		if errors.Is(err, protocol.ErrBadVersion) &&
			headerVersionIncompatible(buf[:n], protocol.CodeHello) {
			sendRejection(protocol.StatusIncompatible)
			return nil, ErrIncompatible
		}
		return nil, wrapErr(ErrProtocol, "hello header: "+err.Error())
	}

	if hdr.Kind != protocol.KindControl || hdr.Code != protocol.CodeHello {
		return nil, wrapErr(ErrProtocol, "expected HELLO")
	}

	hello, err := protocol.DecodeHello(buf[protocol.HeaderSize:n])
	if err != nil {
		if errors.Is(err, protocol.ErrBadLayout) &&
			helloLayoutIncompatible(buf[protocol.HeaderSize:n]) {
			sendRejection(protocol.StatusIncompatible)
			return nil, ErrIncompatible
		}
		return nil, wrapErr(ErrProtocol, "hello payload: "+err.Error())
	}

	intersection := hello.SupportedProfiles & sProfiles

	if intersection == 0 {
		sendRejection(protocol.StatusUnsupported)
		return nil, ErrNoProfile
	}

	if hello.AuthToken != config.AuthToken {
		sendRejection(protocol.StatusAuthFailed)
		return nil, ErrAuthFailed
	}

	// Select profile
	preferredIntersection := intersection & hello.PreferredProfiles & sPreferred
	var selected uint32
	if preferredIntersection != 0 {
		selected = highestBit(preferredIntersection)
	} else {
		selected = highestBit(intersection)
	}

	if hello.MaxRequestPayloadBytes > protocol.MaxPayloadCap {
		sendRejection(protocol.StatusLimitExceeded)
		return nil, ErrLimitExceeded
	}

	// Negotiate limits
	agreedReqPay := hello.MaxRequestPayloadBytes
	agreedReqBat := hello.MaxRequestBatchItems
	agreedRespPay := sRespPay
	agreedRespBat := agreedReqBat
	agreedPkt := minU32(hello.PacketSize, serverPktSize)
	if agreedPkt <= protocol.HeaderSize {
		sendRejection(protocol.StatusIncompatible)
		return nil, ErrIncompatible
	}

	// Send HELLO_ACK
	ack := protocol.HelloAck{
		LayoutVersion:                 1,
		Flags:                         0,
		ServerSupportedProfiles:       sProfiles,
		IntersectionProfiles:          intersection,
		SelectedProfile:               selected,
		AgreedMaxRequestPayloadBytes:  agreedReqPay,
		AgreedMaxRequestBatchItems:    agreedReqBat,
		AgreedMaxResponsePayloadBytes: agreedRespPay,
		AgreedMaxResponseBatchItems:   agreedRespBat,
		AgreedPacketSize:              agreedPkt,
		SessionID:                     sessionID,
	}

	var ackPayBuf [helloAckPayloadSize]byte
	ack.Encode(ackPayBuf[:])

	ackHdr := protocol.Header{
		Magic:           protocol.MagicMsg,
		Version:         protocol.Version,
		HeaderLen:       protocol.HeaderLen,
		Kind:            protocol.KindControl,
		Code:            protocol.CodeHelloAck,
		TransportStatus: protocol.StatusOK,
		PayloadLen:      helloAckPayloadSize,
		ItemCount:       1,
	}

	var pkt [protocol.HeaderSize + helloAckPayloadSize]byte
	ackHdr.Encode(pkt[:protocol.HeaderSize])
	copy(pkt[protocol.HeaderSize:], ackPayBuf[:])

	if err := rawWrite(handle, pkt[:]); err != nil {
		return nil, wrapErr(ErrSend, "hello_ack send: "+err.Error())
	}

	return &Session{
		handle:                  handle,
		role:                    RoleServer,
		MaxRequestPayloadBytes:  agreedReqPay,
		MaxRequestBatchItems:    agreedReqBat,
		MaxResponsePayloadBytes: agreedRespPay,
		MaxResponseBatchItems:   agreedRespBat,
		PacketSize:              agreedPkt,
		SelectedProfile:         selected,
		SessionID:               sessionID,
		inflightIDs:             make(map[uint64]struct{}),
	}, nil
}
