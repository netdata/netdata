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
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
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
	PacketSize              uint32 // 0 = auto-detect from SO_SNDBUF
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
	PacketSize              uint32 // 0 = auto-detect from SO_SNDBUF
	Backlog                 int    // 0 = default (16)
}

// ---------------------------------------------------------------------------
//  Session
// ---------------------------------------------------------------------------

// Session is a connected UDS SEQPACKET session (client or server side).
type Session struct {
	fd   int
	role Role

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

	// Reusable packet scratch buffer for receive chunk assembly.
	pktBuf []byte

	// In-flight message_id set (client-side only)
	inflightIDs map[uint64]struct{}
}

func (s *Session) failAllInflight() {
	if s.role != RoleClient || len(s.inflightIDs) == 0 {
		return
	}
	clear(s.inflightIDs)
}

// Fd returns the raw file descriptor for poll/epoll integration.
func (s *Session) Fd() int {
	return s.fd
}

// Role returns the session role.
func (s *Session) Role() Role {
	return s.role
}

// Close closes the session and releases resources.
func (s *Session) Close() {
	if s.fd >= 0 {
		syscall.Close(s.fd)
		s.fd = -1
	}
	s.recvBuf = nil
	s.pktBuf = nil
	s.failAllInflight()
}

// Connect establishes a session to a server at {runDir}/{serviceName}.sock.
// Performs the full handshake. Blocks until connected + handshake done.
func Connect(runDir, serviceName string, config *ClientConfig) (*Session, error) {
	path, err := buildSocketPath(runDir, serviceName)
	if err != nil {
		return nil, err
	}

	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_SEQPACKET, 0)
	if err != nil {
		return nil, wrapErr(ErrSocket, err.Error())
	}

	session, herr := connectAndHandshake(fd, path, config)
	if herr != nil {
		syscall.Close(fd)
		return nil, herr
	}
	return session, nil
}

// Send sends one logical message. The caller fills Kind, Code, Flags,
// ItemCount, MessageID in hdr; this function sets Magic/Version/
// HeaderLen/PayloadLen. If the total message exceeds PacketSize,
// it is chunked transparently.
func (s *Session) Send(hdr *protocol.Header, payload []byte) error {
	if s.fd < 0 {
		return wrapErr(ErrBadParam, "session closed")
	}

	// Client-side: track in-flight message_ids for requests
	if s.role == RoleClient && hdr.Kind == protocol.KindRequest {
		if s.inflightIDs == nil {
			s.inflightIDs = make(map[uint64]struct{})
		}
		if _, exists := s.inflightIDs[hdr.MessageID]; exists {
			return wrapErr(ErrDuplicateMsgID, fmt.Sprintf("message_id %d", hdr.MessageID))
		}
		s.inflightIDs[hdr.MessageID] = struct{}{}
	}

	// Fill envelope fields
	hdr.Magic = protocol.MagicMsg
	hdr.Version = protocol.Version
	hdr.HeaderLen = protocol.HeaderLen
	hdr.PayloadLen = uint32(len(payload))

	tracked := s.role == RoleClient && hdr.Kind == protocol.KindRequest

	sendErr := s.sendInner(hdr, payload)

	// Rollback: remove message_id from in-flight set on send failure
	if sendErr != nil && tracked {
		if errors.Is(sendErr, ErrSend) {
			s.failAllInflight()
		} else {
			delete(s.inflightIDs, hdr.MessageID)
		}
	}

	return sendErr
}

// sendInner performs the actual send logic, separated so the caller can
// rollback the in-flight set on failure.
func (s *Session) sendInner(hdr *protocol.Header, payload []byte) error {
	totalMsg := protocol.HeaderSize + len(payload)

	// Single packet?
	if totalMsg <= int(s.PacketSize) {
		var hdrBuf [protocol.HeaderSize]byte
		hdr.Encode(hdrBuf[:])
		return rawSendIov(s.fd, hdrBuf[:], payload)
	}

	// Chunked send
	chunkPayloadBudget := int(s.PacketSize) - protocol.HeaderSize
	if chunkPayloadBudget <= 0 {
		return wrapErr(ErrBadParam, "packet_size too small")
	}

	firstChunkPayload := min(len(payload), chunkPayloadBudget)

	remainingAfterFirst := len(payload) - firstChunkPayload
	continuationChunks := uint32(0)
	if remainingAfterFirst > 0 {
		continuationChunks = uint32((remainingAfterFirst + chunkPayloadBudget - 1) / chunkPayloadBudget)
	}
	chunkCount := 1 + continuationChunks

	// Send first chunk: outer header + first part of payload
	var hdrBuf [protocol.HeaderSize]byte
	hdr.Encode(hdrBuf[:])
	if err := rawSendIov(s.fd, hdrBuf[:], payload[:firstChunkPayload]); err != nil {
		return err
	}

	// Send continuation chunks
	offset := firstChunkPayload
	for ci := uint32(1); ci < chunkCount; ci++ {
		remaining := len(payload) - offset
		thisChunk := min(remaining, chunkPayloadBudget)

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

		var chkBuf [protocol.HeaderSize]byte
		chk.Encode(chkBuf[:])
		if err := rawSendIov(s.fd, chkBuf[:], payload[offset:offset+thisChunk]); err != nil {
			return err
		}

		offset += thisChunk
	}

	return nil
}

// Receive reads one logical message. Blocks until a complete message
// arrives. buf is a caller-provided scratch buffer for the first packet.
// On success, returns the header and a payload view valid until the next
// Receive call on this session.
func (s *Session) Receive(buf []byte) (protocol.Header, []byte, error) {
	if s.fd < 0 {
		return protocol.Header{}, nil, wrapErr(ErrBadParam, "session closed")
	}

	// Read first packet
	n, err := rawRecv(s.fd, buf)
	if err != nil {
		if errors.Is(err, ErrRecv) {
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

	// Validate payload_len against negotiated directional limit.
	// Server receives requests; client receives responses.
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

	// Validate item_count against negotiated directional batch limit.
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

	// Client-side: validate response message_id is in-flight
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

	// Non-chunked: entire message in one packet
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

	// Chunked: first packet has partial payload
	firstPayloadBytes := n - protocol.HeaderSize

	// Ensure recv buffer is large enough
	needed := int(hdr.PayloadLen)
	if len(s.recvBuf) < needed {
		s.recvBuf = make([]byte, needed)
	}

	// Copy first chunk's payload
	copy(s.recvBuf[:firstPayloadBytes], buf[protocol.HeaderSize:protocol.HeaderSize+firstPayloadBytes])

	assembled := firstPayloadBytes
	chunkPayloadBudget := int(s.PacketSize) - protocol.HeaderSize

	// Expected chunk count
	remainingAfterFirst := int(hdr.PayloadLen) - firstPayloadBytes
	expectedContinuations := uint32(0)
	if remainingAfterFirst > 0 && chunkPayloadBudget > 0 {
		expectedContinuations = uint32((remainingAfterFirst + chunkPayloadBudget - 1) / chunkPayloadBudget)
	}
	expectedChunkCount := 1 + expectedContinuations

	// Temporary buffer for continuation packets
	pktBuf := ensureScratchBuf(&s.pktBuf, int(s.PacketSize))

	ci := uint32(1)
	for assembled < int(hdr.PayloadLen) {
		cn, err := rawRecv(s.fd, pktBuf)
		if err != nil {
			if errors.Is(err, ErrRecv) {
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

		// Validate chunk header
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

// ---------------------------------------------------------------------------
//  Listener
// ---------------------------------------------------------------------------

// Listener is a listening UDS SEQPACKET endpoint.
type Listener struct {
	fd            int
	config        ServerConfig
	path          string
	nextSessionID atomic.Uint64
}

// Listen creates a listener on {runDir}/{serviceName}.sock.
// Performs stale endpoint recovery.
func Listen(runDir, serviceName string, config ServerConfig) (*Listener, error) {
	path, err := buildSocketPath(runDir, serviceName)
	if err != nil {
		return nil, err
	}

	// Stale recovery
	stale := checkAndRecoverStale(path)
	if stale == staleLiveServer {
		return nil, ErrAddrInUse
	}

	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_SEQPACKET, 0)
	if err != nil {
		return nil, wrapErr(ErrSocket, err.Error())
	}

	// Bind
	sa := &syscall.SockaddrUnix{Name: path}
	if err := syscall.Bind(fd, sa); err != nil {
		syscall.Close(fd)
		return nil, wrapErr(ErrSocket, "bind: "+err.Error())
	}

	backlog := config.Backlog
	if backlog <= 0 {
		backlog = defaultBacklog
	}

	if err := syscall.Listen(fd, backlog); err != nil {
		syscall.Close(fd)
		os.Remove(path)
		return nil, wrapErr(ErrSocket, "listen: "+err.Error())
	}

	return &Listener{
		fd:     fd,
		config: config,
		path:   path,
	}, nil
}

// Fd returns the raw file descriptor for poll/epoll integration.
func (l *Listener) Fd() int {
	return l.fd
}

// SetPayloadLimits updates the payload limits used for future handshakes.
func (l *Listener) SetPayloadLimits(maxRequestPayloadBytes, maxResponsePayloadBytes uint32) {
	l.config.MaxRequestPayloadBytes = maxRequestPayloadBytes
	l.config.MaxResponsePayloadBytes = maxResponsePayloadBytes
}

// Accept accepts one client connection. Performs the full handshake.
// Blocks until a client connects and the handshake completes.
func (l *Listener) Accept() (*Session, error) {
	sessionID := l.nextSessionID.Add(1)
	return l.AcceptWithConfig(sessionID, l.config)
}

// AcceptWithConfig accepts one client connection using a caller-provided
// per-session server config and session ID.
func (l *Listener) AcceptWithConfig(sessionID uint64, config ServerConfig) (*Session, error) {
	nfd, _, err := syscall.Accept(l.fd)
	if err != nil {
		return nil, wrapErr(ErrAccept, err.Error())
	}

	session, herr := serverHandshake(nfd, &config, sessionID)
	if herr != nil {
		syscall.Close(nfd)
		return nil, herr
	}
	return session, nil
}

// Close closes the listener, stops accepting, and unlinks the socket file.
func (l *Listener) Close() {
	if l.fd >= 0 {
		syscall.Close(l.fd)
		l.fd = -1
	}
	if l.path != "" {
		os.Remove(l.path)
		l.path = ""
	}
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

func maxU32(a, b uint32) uint32 {
	if a > b {
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
	iov[0].Base = unsafe.SliceData(hdr)
	iov[0].SetLen(len(hdr))

	iovlen := uint64(1)
	if len(payload) > 0 {
		iov[1].Base = unsafe.SliceData(payload)
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
		uintptr(unsafe.Pointer(&msg)),
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

// ---------------------------------------------------------------------------
//  Stale endpoint recovery
// ---------------------------------------------------------------------------

type staleResult int

const (
	staleNotExist   staleResult = 0
	staleRecovered  staleResult = 1
	staleLiveServer staleResult = 2
)

func checkAndRecoverStale(path string) staleResult {
	_, err := os.Stat(path)
	if err != nil {
		return staleNotExist
	}

	// Try connecting to see if a live server is there.
	// We use net.Dial instead of raw syscalls for the probe — it handles
	// all the sockaddr setup and is fine for a one-shot connectivity test.
	conn, err := net.Dial("unixpacket", path)
	if err == nil {
		// Connected => live server
		conn.Close()
		return staleLiveServer
	}

	// Only unlink on connection-refused (stale socket).
	// Other errors (EACCES, etc.) should not remove the file.
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ENOENT) {
		os.Remove(path)
		return staleRecovered
	}
	// Can't determine ownership — treat as live to prevent overwriting
	return staleLiveServer
}

// ---------------------------------------------------------------------------
//  Handshake: client side
// ---------------------------------------------------------------------------

func connectAndHandshake(fd int, path string, config *ClientConfig) (*Session, error) {
	// Connect
	sa := &syscall.SockaddrUnix{Name: path}
	if err := syscall.Connect(fd, sa); err != nil {
		return nil, wrapErr(ErrConnect, err.Error())
	}

	pktSize := config.PacketSize
	if pktSize == 0 {
		pktSize = detectPacketSize(fd)
	}

	supported := config.SupportedProfiles
	if supported == 0 {
		supported = protocol.ProfileBaseline
	}

	// Build HELLO
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

	// Build outer CONTROL header
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
	n, err := syscall.SendmsgN(fd, pkt[:], nil, nil, 0)
	if err != nil {
		return nil, wrapErr(ErrSend, "hello send: "+err.Error())
	}
	if n != len(pkt) {
		return nil, wrapErr(ErrSend, "hello short write")
	}

	// Receive HELLO_ACK
	var ackBuf [128]byte
	an, _, _, _, err := syscall.Recvmsg(fd, ackBuf[:], nil, 0)
	if err != nil {
		return nil, wrapErr(ErrRecv, "hello_ack recv: "+err.Error())
	}
	if an == 0 {
		return nil, wrapErr(ErrRecv, "peer disconnected during handshake")
	}

	// Decode outer header
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

	// Check transport_status for rejection
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

	// Decode hello-ack payload
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
		fd:                      fd,
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
//  Handshake: server side
// ---------------------------------------------------------------------------

func serverHandshake(fd int, config *ServerConfig, sessionID uint64) (*Session, error) {
	serverPktSize := config.PacketSize
	if serverPktSize == 0 {
		serverPktSize = detectPacketSize(fd)
	}

	sRespPay := applyDefault(config.MaxResponsePayloadBytes, protocol.MaxPayloadDefault)
	sProfiles := config.SupportedProfiles
	if sProfiles == 0 {
		sProfiles = protocol.ProfileBaseline
	}
	sPreferred := config.PreferredProfiles

	// Helper: send rejection ACK
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
		// Best effort send
		syscall.SendmsgN(fd, pkt[:], nil, nil, 0) //nolint:errcheck
	}

	// Receive HELLO
	var buf [128]byte
	n, _, _, _, err := syscall.Recvmsg(fd, buf[:], nil, 0)
	if err != nil {
		return nil, wrapErr(ErrRecv, "hello recv: "+err.Error())
	}
	if n == 0 {
		return nil, wrapErr(ErrRecv, "peer disconnected during handshake")
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

	// Compute intersection
	intersection := hello.SupportedProfiles & sProfiles

	// Check intersection
	if intersection == 0 {
		sendRejection(protocol.StatusUnsupported)
		return nil, ErrNoProfile
	}

	// Check auth
	if hello.AuthToken != config.AuthToken {
		sendRejection(protocol.StatusAuthFailed)
		return nil, ErrAuthFailed
	}

	// Select profile: prefer preferred_intersection, then intersection
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

	// Negotiate limits:
	// - request payload and batch size are client-proposed and echoed
	// - response payload is server-authoritative
	// - response batch size is symmetric with request batch size
	agreedReqPay := hello.MaxRequestPayloadBytes
	agreedReqBat := hello.MaxRequestBatchItems
	agreedRespPay := sRespPay
	agreedRespBat := agreedReqBat
	agreedPkt := minU32(hello.PacketSize, serverPktSize)
	if agreedPkt <= protocol.HeaderSize {
		sendRejection(protocol.StatusIncompatible)
		return nil, ErrIncompatible
	}

	// Send HELLO_ACK (success)
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

	sn, err := syscall.SendmsgN(fd, pkt[:], nil, nil, 0)
	if err != nil {
		return nil, wrapErr(ErrSend, "hello_ack send: "+err.Error())
	}
	if sn != len(pkt) {
		return nil, wrapErr(ErrSend, "hello_ack short write")
	}

	return &Session{
		fd:                      fd,
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
