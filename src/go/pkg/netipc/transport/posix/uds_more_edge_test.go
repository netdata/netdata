//go:build unix

package posix

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/internal/monotime"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

func udsSessionPair(t *testing.T, sCfg ServerConfig, cCfg ClientConfig) (*Session, *Session) {
	t.Helper()

	runDir := testRunDir(t)
	service := uniqueService(t)
	socketPath := filepath.Join(runDir, service+".sock")
	t.Cleanup(func() { _ = os.Remove(socketPath) })

	listener := startListener(t, runDir, service, sCfg)
	t.Cleanup(listener.Close)

	acceptCh := acceptAsync(listener)

	client, err := Connect(runDir, service, &cCfg)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	t.Cleanup(client.Close)

	sr := <-acceptCh
	if sr.err != nil {
		t.Fatalf("Accept failed: %v", sr.err)
	}
	server := sr.session
	t.Cleanup(server.Close)

	return client, server
}

func rawSendPacket(fd int, pkt []byte) error {
	n, err := syscall.SendmsgN(fd, pkt, nil, nil, 0)
	if err != nil {
		return err
	}
	if n != len(pkt) {
		return fmt.Errorf("short write: %d/%d", n, len(pkt))
	}
	return nil
}

// endStreamWrites: on a stream a short or over-long packet is only visible
// once the peer stops sending, so end the peer's writes there; the receiver
// then sees EOF where a SEQPACKET receiver sees the packet end.
func endStreamWrites(fd int) {
	if streamTransport {
		_ = syscall.Shutdown(fd, syscall.SHUT_WR)
	}
}

func encodePacket(hdr protocol.Header, payload []byte) []byte {
	hdr.Magic = protocol.MagicMsg
	hdr.Version = protocol.Version
	hdr.HeaderLen = protocol.HeaderLen
	hdr.PayloadLen = uint32(len(payload))

	pkt := make([]byte, protocol.HeaderSize+len(payload))
	hdr.Encode(pkt[:protocol.HeaderSize])
	copy(pkt[protocol.HeaderSize:], payload)
	return pkt
}

func encodePartialPacket(hdr protocol.Header, advertisedPayloadLen uint32, payload []byte) []byte {
	hdr.Magic = protocol.MagicMsg
	hdr.Version = protocol.Version
	hdr.HeaderLen = protocol.HeaderLen
	hdr.PayloadLen = advertisedPayloadLen

	pkt := make([]byte, protocol.HeaderSize+len(payload))
	hdr.Encode(pkt[:protocol.HeaderSize])
	copy(pkt[protocol.HeaderSize:], payload)
	return pkt
}

func encodeChunkPacket(chk protocol.ChunkHeader, payload []byte) []byte {
	pkt := make([]byte, protocol.HeaderSize+len(payload))
	chk.Encode(pkt[:protocol.HeaderSize])
	copy(pkt[protocol.HeaderSize:], payload)
	return pkt
}

func validHelloAckPacket(status uint16) []byte {
	ack := protocol.HelloAck{
		LayoutVersion:                 1,
		Flags:                         0,
		ServerSupportedProfiles:       protocol.ProfileBaseline,
		IntersectionProfiles:          protocol.ProfileBaseline,
		SelectedProfile:               protocol.ProfileBaseline,
		AgreedMaxRequestPayloadBytes:  protocol.MaxPayloadDefault,
		AgreedMaxRequestBatchItems:    1,
		AgreedMaxResponsePayloadBytes: protocol.MaxPayloadDefault,
		AgreedMaxResponseBatchItems:   1,
		AgreedPacketSize:              defaultPacketSizeFallback,
		SessionID:                     77,
	}

	payload := make([]byte, helloAckPayloadSize)
	ack.Encode(payload)
	return encodePacket(protocol.Header{
		Kind:            protocol.KindControl,
		Code:            protocol.CodeHelloAck,
		TransportStatus: status,
		ItemCount:       1,
	}, payload)
}

// helloAckSelecting is a valid HELLO_ACK whose selected_profile is profile.
func helloAckSelecting(profile uint32) []byte {
	ack := validHelloAckPacket(protocol.StatusOK)
	binary.NativeEndian.PutUint32(ack[protocol.HeaderSize+12:protocol.HeaderSize+16], profile)
	return ack
}

func validHelloPacket() []byte {
	hello := protocol.Hello{
		LayoutVersion:           1,
		Flags:                   0,
		SupportedProfiles:       protocol.ProfileBaseline,
		PreferredProfiles:       protocol.ProfileBaseline,
		MaxRequestPayloadBytes:  protocol.MaxPayloadDefault,
		MaxRequestBatchItems:    1,
		MaxResponsePayloadBytes: protocol.MaxPayloadDefault,
		MaxResponseBatchItems:   1,
		AuthToken:               testAuthToken,
		PacketSize:              defaultPacketSizeFallback,
	}

	payload := make([]byte, helloPayloadSize)
	hello.Encode(payload)
	return encodePacket(protocol.Header{
		Kind:      protocol.KindControl,
		Code:      protocol.CodeHello,
		ItemCount: 1,
	}, payload)
}

func TestHelloAckStatusErrorsAndSendGuard(t *testing.T) {
	for _, tc := range []struct {
		status uint16
		want   error
	}{
		{status: protocol.StatusOK, want: nil},
		{status: protocol.StatusAuthFailed, want: ErrAuthFailed},
		{status: protocol.StatusUnsupported, want: ErrNoProfile},
		{status: protocol.StatusIncompatible, want: ErrIncompatible},
		{status: protocol.StatusLimitExceeded, want: ErrLimitExceeded},
		{status: 0xffff, want: ErrHandshake},
	} {
		err := helloAckStatusError(tc.status)
		if !errors.Is(err, tc.want) {
			t.Fatalf("helloAckStatusError(%d) = %v, want %v", tc.status, err, tc.want)
		}
	}

	if err := sendHelloAck(-1, protocol.StatusOK, protocol.HelloAck{}); err == nil {
		t.Fatal("sendHelloAck with invalid fd should fail")
	}
}

func rawSeqpacketListener(t *testing.T, runDir, service string) (int, string) {
	t.Helper()

	path, err := buildSocketPath(runDir, service)
	if err != nil {
		t.Fatalf("buildSocketPath failed: %v", err)
	}
	_ = os.Remove(path)

	fd, err := syscall.Socket(syscall.AF_UNIX, sockType, 0)
	if err != nil {
		t.Fatalf("socket failed: %v", err)
	}
	t.Cleanup(func() {
		_ = syscall.Close(fd)
		_ = os.Remove(path)
	})

	if err := syscall.Bind(fd, &syscall.SockaddrUnix{Name: path}); err != nil {
		t.Fatalf("bind failed: %v", err)
	}
	if err := syscall.Listen(fd, defaultBacklog); err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	return fd, path
}

func rawSeqpacketConnect(t *testing.T, path string) int {
	t.Helper()

	fd, err := syscall.Socket(syscall.AF_UNIX, sockType, 0)
	if err != nil {
		t.Fatalf("socket failed: %v", err)
	}
	t.Cleanup(func() { _ = syscall.Close(fd) })

	if err := syscall.Connect(fd, &syscall.SockaddrUnix{Name: path}); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	return fd
}

func TestUdsAcceptOnClosedListener(t *testing.T) {
	runDir := testRunDir(t)
	service := uniqueService(t)
	listener := startListener(t, runDir, service, defaultServerConfig())
	listener.Close()

	session, err := listener.Accept()
	if session != nil {
		session.Close()
	}
	if !errors.Is(err, ErrAccept) {
		t.Fatalf("Accept after close = %v, want ErrAccept", err)
	}
}

func TestUdsListenFailsWhenRunDirMissing(t *testing.T) {
	runDir := filepath.Join(shortTempDir(t), "missing")
	_, err := Listen(runDir, uniqueService(t), defaultServerConfig())
	if !errors.Is(err, ErrSocket) {
		t.Fatalf("Listen(missing runDir) = %v, want ErrSocket", err)
	}
	if !containsErrText(err, "lock open:") {
		t.Fatalf("Listen(missing runDir) = %v, want lock open failure detail", err)
	}
}

func TestUdsRawGenericErrors(t *testing.T) {
	if err := rawSendIov(-1, []byte("x"), nil); !errors.Is(err, ErrSend) {
		t.Fatalf("rawSendIov(invalid fd) = %v, want ErrSend", err)
	}

	buf := make([]byte, 16)
	if _, err := rawRecv(-1, buf); !errors.Is(err, ErrRecv) {
		t.Fatalf("rawRecv(invalid fd) = %v, want ErrRecv", err)
	}
}

func TestReceiveWaitTimeoutAndAbort(t *testing.T) {
	abortCh := make(chan struct{})
	close(abortCh)
	if _, err := newReceiveWait(1000, abortCh).waitMs(); !errors.Is(err, ErrAborted) {
		t.Fatalf("waitMs aborted = %v, want ErrAborted", err)
	}

	if _, err := newReceiveWait(1, nil).waitMs(); err != nil {
		t.Fatalf("waitMs before deadline = %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := newReceiveWait(1, nil).waitMs(); err != nil {
		t.Fatalf("fresh waitMs should reset deadline: %v", err)
	}

	expired := receiveWait{deadline: monotime.After(-time.Millisecond)}
	if _, err := expired.waitMs(); !errors.Is(err, ErrTimeout) {
		t.Fatalf("waitMs expired = %v, want ErrTimeout", err)
	}

	infinite := newReceiveWait(0, nil)
	if waitMs, err := infinite.waitMs(); err != nil || waitMs != receiveAbortPollMs {
		t.Fatalf("waitMs infinite = %d/%v, want %d/nil", waitMs, err, receiveAbortPollMs)
	}

	short := receiveWait{deadline: monotime.After(1500 * time.Microsecond)}
	if waitMs, err := short.waitMs(); err != nil || waitMs < 1 || waitMs > 2 {
		t.Fatalf("waitMs short deadline = %d/%v, want 1..2/nil", waitMs, err)
	}
}

func TestReceiveAndStaleDirectGuards(t *testing.T) {
	// Non-constant so 32-bit builds compile; there it wraps negative and still hits the guard.
	var highFD64 int64 = 1 << 31
	highFD := int(highFD64)
	if err := pollReadableForReceive(highFD, newReceiveWait(1, nil)); !errors.Is(err, ErrBadParam) {
		t.Fatalf("poll high fd error = %v, want ErrBadParam", err)
	}
	if _, err := rawRecvWithTimeout(highFD, make([]byte, 1), newReceiveWait(1, nil)); !errors.Is(err, ErrBadParam) {
		t.Fatalf("recv high fd error = %v, want ErrBadParam", err)
	}

	closedFD, err := syscall.Socket(syscall.AF_UNIX, sockType, 0)
	if err != nil {
		t.Fatalf("socket for closed-fd receive guard: %v", err)
	}
	if err := syscall.Close(closedFD); err != nil {
		t.Fatalf("close guard socket: %v", err)
	}
	if err := pollReadableForReceive(closedFD, newReceiveWait(1, nil)); err != nil {
		t.Fatalf("poll closed fd error = %v, want nil poll readiness for HUP/NVAL", err)
	}
	if _, err := rawRecvWithTimeout(closedFD, make([]byte, 1), newReceiveWait(1, nil)); !errors.Is(err, ErrRecv) {
		t.Fatalf("recv closed fd error = %v, want ErrRecv", err)
	}

	missing := filepath.Join(t.TempDir(), "missing.sock")
	if got := checkAndRecoverStale(missing); got != staleNotExist {
		t.Fatalf("missing stale result = %d, want staleNotExist", got)
	}
	if err := dialStaleCandidate(missing); err == nil {
		t.Fatal("dial missing stale candidate should fail")
	}

	runDir := testRunDir(t)
	service := uniqueService(t)
	lfd, livePath := rawSeqpacketListener(t, runDir, service)
	defer syscall.Close(lfd)
	if got := checkAndRecoverStale(livePath); got != staleLiveServer {
		t.Fatalf("live stale result = %d, want staleLiveServer", got)
	}

	stalePath := filepath.Join(t.TempDir(), "stale.sock")
	if err := os.WriteFile(stalePath, []byte("not a socket"), 0o600); err != nil {
		t.Fatalf("write stale endpoint placeholder: %v", err)
	}
	if got := checkAndRecoverStale(stalePath); got != staleRecovered {
		t.Fatalf("regular-file stale result = %d, want staleRecovered", got)
	}
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatalf("regular-file stale path still exists: %v", err)
	}
}

func TestReceiveTimeoutFailurePaths(t *testing.T) {
	client, _ := udsSessionPair(t, defaultServerConfig(), defaultClientConfig())
	defer client.Close()

	buf := make([]byte, 4096)
	if _, _, err := client.ReceiveTimeout(buf, 1, nil); !errors.Is(err, ErrTimeout) {
		t.Fatalf("ReceiveTimeout timeout = %v, want ErrTimeout", err)
	}

	abortCh := make(chan struct{})
	close(abortCh)
	if _, _, err := client.ReceiveTimeout(buf, 1000, abortCh); !errors.Is(err, ErrAborted) {
		t.Fatalf("ReceiveTimeout aborted = %v, want ErrAborted", err)
	}

	if err := pollReadableForReceive(-1, newReceiveWait(1, nil)); !errors.Is(err, ErrBadParam) {
		t.Fatalf("pollReadableForReceive bad fd = %v, want ErrBadParam", err)
	}

	client.Close()
	if _, _, err := client.ReceiveTimeout(buf, 1, nil); !errors.Is(err, ErrBadParam) {
		t.Fatalf("ReceiveTimeout closed session = %v, want ErrBadParam", err)
	}
}

func TestUdsSessionSendRejectsTooSmallPacketSize(t *testing.T) {
	client, _ := udsSessionPair(t, defaultServerConfig(), defaultClientConfig())

	client.PacketSize = uint32(protocol.HeaderSize)
	hdr := protocol.Header{
		Kind:      protocol.KindRequest,
		Code:      protocol.MethodIncrement,
		ItemCount: 1,
		MessageID: 1,
	}
	if err := client.Send(&hdr, []byte("x")); !errors.Is(err, ErrBadParam) {
		t.Fatalf("Send(packet_size too small) = %v, want ErrBadParam", err)
	}
}

func TestUdsSendInitializesInflightSetOnFirstRequest(t *testing.T) {
	client, server := udsSessionPair(t, defaultServerConfig(), defaultClientConfig())

	client.inflightIDs = nil
	payload := []byte("ping")
	hdr := protocol.Header{
		Kind:      protocol.KindRequest,
		Code:      protocol.MethodIncrement,
		ItemCount: 1,
		MessageID: 55,
	}

	if err := client.Send(&hdr, payload); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if client.inflightIDs == nil {
		t.Fatal("Send should initialize inflightIDs for the first client request")
	}
	if _, ok := client.inflightIDs[55]; !ok {
		t.Fatal("Send should track the in-flight message_id")
	}

	buf := make([]byte, 4096)
	rHdr, rPayload, err := server.Receive(buf)
	if err != nil {
		t.Fatalf("server Receive failed: %v", err)
	}
	if rHdr.MessageID != 55 {
		t.Fatalf("server received message_id=%d, want 55", rHdr.MessageID)
	}
	if string(rPayload) != string(payload) {
		t.Fatalf("server received payload=%q, want %q", rPayload, payload)
	}
}

func TestUdsReceiveRejectsMalformedFirstPacket(t *testing.T) {
	cases := []struct {
		name string
		pkt  []byte
		want error
	}{
		{
			name: "short packet",
			pkt:  []byte{1, 2, 3},
			want: ErrProtocol,
		},
		{
			name: "bad header",
			pkt:  make([]byte, protocol.HeaderSize),
			want: ErrProtocol,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, server := udsSessionPair(t, defaultServerConfig(), defaultClientConfig())

			client.inflightIDs[42] = struct{}{}
			if err := rawSendPacket(server.fd, tc.pkt); err != nil {
				t.Fatalf("rawSendPacket failed: %v", err)
			}
			endStreamWrites(server.fd)

			buf := make([]byte, 4096)
			_, _, err := client.Receive(buf)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Receive error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestUdsReceiveRejectsUnknownMessageIDWithNilInflightSet(t *testing.T) {
	client, server := udsSessionPair(t, defaultServerConfig(), defaultClientConfig())

	client.inflightIDs = nil
	respPkt := encodePacket(protocol.Header{
		Kind:      protocol.KindResponse,
		Code:      protocol.MethodIncrement,
		ItemCount: 1,
		MessageID: 42,
	}, []byte("ok"))
	if err := rawSendPacket(server.fd, respPkt); err != nil {
		t.Fatalf("rawSendPacket failed: %v", err)
	}

	buf := make([]byte, 4096)
	_, _, err := client.Receive(buf)
	if !errors.Is(err, ErrUnknownMsgID) {
		t.Fatalf("Receive error = %v, want ErrUnknownMsgID", err)
	}
}

func TestUdsReceiveRejectsMalformedBatchPayload(t *testing.T) {
	client, server := udsSessionPair(t, defaultServerConfig(), defaultClientConfig())

	client.inflightIDs[42] = struct{}{}
	payload := make([]byte, 24)
	binary.NativeEndian.PutUint32(payload[0:4], 1)
	binary.NativeEndian.PutUint32(payload[4:8], 4)
	binary.NativeEndian.PutUint32(payload[8:12], 0)
	binary.NativeEndian.PutUint32(payload[12:16], 4)
	copy(payload[16:], []byte("payload!!"))

	respPkt := encodePacket(protocol.Header{
		Kind:      protocol.KindResponse,
		Code:      protocol.MethodIncrement,
		Flags:     protocol.FlagBatch,
		ItemCount: 2,
		MessageID: 42,
	}, payload)
	if err := rawSendPacket(server.fd, respPkt); err != nil {
		t.Fatalf("rawSendPacket failed: %v", err)
	}

	buf := make([]byte, 4096)
	_, _, err := client.Receive(buf)
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("Receive error = %v, want ErrProtocol", err)
	}
}

func TestUdsReceiveRejectsMalformedChunkedBatchPayload(t *testing.T) {
	cases := []struct {
		name    string
		payload []byte
		first   int
	}{
		{
			name:    "batch dir exceeds payload",
			payload: make([]byte, 8),
			first:   4,
		},
		{
			name: "batch dir invalid after reassembly",
			payload: func() []byte {
				payload := make([]byte, 24)
				binary.NativeEndian.PutUint32(payload[0:4], 1)
				binary.NativeEndian.PutUint32(payload[4:8], 4)
				binary.NativeEndian.PutUint32(payload[8:12], 0)
				binary.NativeEndian.PutUint32(payload[12:16], 4)
				copy(payload[16:], []byte("payload!!"))
				return payload
			}(),
			first: 12,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, server := udsSessionPair(t, defaultServerConfig(), defaultClientConfig())
			// The first packet is full, as a sender would make it: a stream
			// receiver reads min(payload_len, packet_size - 32) bytes after
			// its header.
			client.PacketSize = uint32(protocol.HeaderSize + tc.first)

			client.inflightIDs[42] = struct{}{}
			firstPkt := encodePartialPacket(protocol.Header{
				Kind:      protocol.KindResponse,
				Code:      protocol.MethodIncrement,
				Flags:     protocol.FlagBatch,
				ItemCount: 2,
				MessageID: 42,
			}, uint32(len(tc.payload)), tc.payload[:tc.first])
			if err := rawSendPacket(server.fd, firstPkt); err != nil {
				t.Fatalf("rawSendPacket(first) failed: %v", err)
			}

			second := tc.payload[tc.first:]
			if len(second) > 0 {
				secondPkt := encodeChunkPacket(protocol.ChunkHeader{
					Magic:           protocol.MagicChunk,
					Version:         protocol.Version,
					MessageID:       42,
					TotalMessageLen: uint32(protocol.HeaderSize + len(tc.payload)),
					ChunkIndex:      1,
					ChunkCount:      2,
					ChunkPayloadLen: uint32(len(second)),
				}, second)
				if err := rawSendPacket(server.fd, secondPkt); err != nil {
					t.Fatalf("rawSendPacket(second) failed: %v", err)
				}
			}

			buf := make([]byte, 4096)
			_, _, err := client.Receive(buf)
			if !errors.Is(err, ErrProtocol) {
				t.Fatalf("Receive error = %v, want ErrProtocol", err)
			}
		})
	}
}

func TestUdsReceiveRejectsMalformedChunks(t *testing.T) {
	cases := []struct {
		name    string
		second  []byte
		want    error
		wantMsg string
	}{
		{
			name:   "continuation too short",
			second: []byte{1, 2, 3},
			want:   ErrChunk,
		},
		{
			name: "bad chunk header",
			second: encodeChunkPacket(protocol.ChunkHeader{
				Magic:           protocol.MagicChunk,
				Version:         protocol.Version + 1,
				MessageID:       42,
				TotalMessageLen: uint32(protocol.HeaderSize + 8),
				ChunkIndex:      1,
				ChunkCount:      2,
				ChunkPayloadLen: 4,
			}, []byte("tail")),
			want: ErrChunk,
		},
		{
			name: "message id mismatch",
			second: encodeChunkPacket(protocol.ChunkHeader{
				Magic:           protocol.MagicChunk,
				Version:         protocol.Version,
				MessageID:       99,
				TotalMessageLen: uint32(protocol.HeaderSize + 8),
				ChunkIndex:      1,
				ChunkCount:      2,
				ChunkPayloadLen: 4,
			}, []byte("tail")),
			want: ErrChunk,
		},
		{
			name: "chunk index mismatch",
			second: encodeChunkPacket(protocol.ChunkHeader{
				Magic:           protocol.MagicChunk,
				Version:         protocol.Version,
				MessageID:       42,
				TotalMessageLen: uint32(protocol.HeaderSize + 8),
				ChunkIndex:      2,
				ChunkCount:      2,
				ChunkPayloadLen: 4,
			}, []byte("tail")),
			want: ErrChunk,
		},
		{
			name: "chunk count mismatch",
			second: encodeChunkPacket(protocol.ChunkHeader{
				Magic:           protocol.MagicChunk,
				Version:         protocol.Version,
				MessageID:       42,
				TotalMessageLen: uint32(protocol.HeaderSize + 8),
				ChunkIndex:      1,
				ChunkCount:      3,
				ChunkPayloadLen: 4,
			}, []byte("tail")),
			want: ErrChunk,
		},
		{
			name: "total len mismatch",
			second: encodeChunkPacket(protocol.ChunkHeader{
				Magic:           protocol.MagicChunk,
				Version:         protocol.Version,
				MessageID:       42,
				TotalMessageLen: uint32(protocol.HeaderSize + 9),
				ChunkIndex:      1,
				ChunkCount:      2,
				ChunkPayloadLen: 4,
			}, []byte("tail")),
			want: ErrChunk,
		},
		{
			name: "chunk payload len mismatch",
			second: encodeChunkPacket(protocol.ChunkHeader{
				Magic:           protocol.MagicChunk,
				Version:         protocol.Version,
				MessageID:       42,
				TotalMessageLen: uint32(protocol.HeaderSize + 8),
				ChunkIndex:      1,
				ChunkCount:      2,
				ChunkPayloadLen: 3,
			}, []byte("tail")),
			want: ErrChunk,
		},
		{
			name: "chunk exceeds payload",
			second: encodeChunkPacket(protocol.ChunkHeader{
				Magic:           protocol.MagicChunk,
				Version:         protocol.Version,
				MessageID:       42,
				TotalMessageLen: uint32(protocol.HeaderSize + 8),
				ChunkIndex:      1,
				ChunkCount:      2,
				ChunkPayloadLen: 5,
			}, []byte("tails")),
			want: ErrChunk,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, server := udsSessionPair(t, defaultServerConfig(), defaultClientConfig())
			// A full first packet carries the 4 bytes of "head".
			client.PacketSize = protocol.HeaderSize + 4

			client.inflightIDs[42] = struct{}{}
			firstPkt := encodePartialPacket(protocol.Header{
				Kind:      protocol.KindResponse,
				Code:      protocol.MethodIncrement,
				ItemCount: 1,
				MessageID: 42,
			}, 8, []byte("head"))
			if err := rawSendPacket(server.fd, firstPkt); err != nil {
				t.Fatalf("rawSendPacket(first) failed: %v", err)
			}
			if err := rawSendPacket(server.fd, tc.second); err != nil {
				t.Fatalf("rawSendPacket(second) failed: %v", err)
			}
			endStreamWrites(server.fd)

			buf := make([]byte, 4096)
			_, _, err := client.Receive(buf)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Receive error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestUdsReceiveRejectsMoreChunksThanDeclared(t *testing.T) {
	client, server := udsSessionPair(t, defaultServerConfig(), defaultClientConfig())
	// A full first packet carries 4 of the 8 payload bytes, so one
	// continuation is declared; two half-budget ones follow.
	client.PacketSize = protocol.HeaderSize + 4

	client.inflightIDs[42] = struct{}{}
	firstPkt := encodePartialPacket(protocol.Header{
		Kind:      protocol.KindResponse,
		Code:      protocol.MethodIncrement,
		ItemCount: 1,
		MessageID: 42,
	}, 8, []byte("head"))
	if err := rawSendPacket(server.fd, firstPkt); err != nil {
		t.Fatalf("rawSendPacket(first) failed: %v", err)
	}
	for i, part := range []string{"ta", "il"} {
		pkt := encodeChunkPacket(protocol.ChunkHeader{
			Magic:           protocol.MagicChunk,
			Version:         protocol.Version,
			MessageID:       42,
			TotalMessageLen: uint32(protocol.HeaderSize + 8),
			ChunkIndex:      uint32(i + 1),
			ChunkCount:      2,
			ChunkPayloadLen: uint32(len(part)),
		}, []byte(part))
		if err := rawSendPacket(server.fd, pkt); err != nil {
			t.Fatalf("rawSendPacket(continuation %d) failed: %v", i+1, err)
		}
	}
	endStreamWrites(server.fd)

	buf := make([]byte, 4096)
	_, _, err := client.Receive(buf)
	if !errors.Is(err, ErrChunk) {
		t.Fatalf("Receive error = %v, want ErrChunk", err)
	}
}

func TestUdsClientHandshakeRejectsMalformedAck(t *testing.T) {
	cases := []struct {
		name     string
		packetFn func() []byte
		want     error
		wantText string
	}{
		{
			name: "bad ack header",
			packetFn: func() []byte {
				ack := validHelloAckPacket(protocol.StatusOK)
				ack[0] = 0
				return ack
			},
			want:     ErrProtocol,
			wantText: "ack header",
		},
		{
			name: "wrong kind",
			packetFn: func() []byte {
				payload := make([]byte, helloAckPayloadSize)
				return encodePacket(protocol.Header{
					Kind:            protocol.KindRequest,
					Code:            protocol.CodeHelloAck,
					TransportStatus: protocol.StatusOK,
					ItemCount:       1,
				}, payload)
			},
			want:     ErrProtocol,
			wantText: "expected HELLO_ACK",
		},
		{
			name: "unexpected status",
			packetFn: func() []byte {
				return validHelloAckPacket(999)
			},
			want:     ErrHandshake,
			wantText: "transport_status=999",
		},
		{
			name: "truncated payload",
			packetFn: func() []byte {
				ack := validHelloAckPacket(protocol.StatusOK)
				return ack[:protocol.HeaderSize+8]
			},
			want:     ErrProtocol,
			wantText: truncatedText("ack payload truncated"),
		},
		{
			name: "agreed packet size above proposal",
			packetFn: func() []byte {
				ack := validHelloAckPacket(protocol.StatusOK)
				binary.NativeEndian.PutUint32(ack[protocol.HeaderSize+32:protocol.HeaderSize+36], ^uint32(0))
				return ack
			},
			want:     ErrProtocol,
			wantText: "agreed packet_size",
		},
		{
			name: "agreed packet size too small",
			packetFn: func() []byte {
				ack := validHelloAckPacket(protocol.StatusOK)
				binary.NativeEndian.PutUint32(ack[protocol.HeaderSize+32:protocol.HeaderSize+36], protocol.HeaderSize)
				return ack
			},
			want:     ErrProtocol,
			wantText: "agreed packet_size",
		},
		{
			name: "bad ack payload layout",
			packetFn: func() []byte {
				ack := validHelloAckPacket(protocol.StatusOK)
				binary.NativeEndian.PutUint16(ack[protocol.HeaderSize:protocol.HeaderSize+2], 2)
				return ack
			},
			want:     ErrIncompatible,
			wantText: "ack payload",
		},
		{
			name: "selected profile not offered",
			packetFn: func() []byte {
				return helloAckSelecting(protocol.ProfileSHMHybrid)
			},
			want:     ErrProtocol,
			wantText: "selected profile was not offered",
		},
		{
			name: "no selected profile",
			packetFn: func() []byte {
				return helloAckSelecting(0)
			},
			want:     ErrProtocol,
			wantText: "selected profile was not offered",
		},
		{
			name: "two selected profiles",
			packetFn: func() []byte {
				return helloAckSelecting(protocol.ProfileBaseline | protocol.ProfileSHMHybrid)
			},
			want:     ErrProtocol,
			wantText: "selected profile was not offered",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runDir := testRunDir(t)
			service := uniqueService(t)
			lfd, path := rawSeqpacketListener(t, runDir, service)

			serverDone := make(chan error, 1)
			go func() {
				nfd, _, err := syscall.Accept(lfd)
				if err != nil {
					serverDone <- err
					return
				}
				defer syscall.Close(nfd)

				var helloBuf [128]byte
				if _, err := rawRecv(nfd, helloBuf[:]); err != nil {
					serverDone <- err
					return
				}
				serverDone <- rawSendPacket(nfd, tc.packetFn())
			}()

			client, err := Connect(runDir, service, defaultClientConfigPtr())
			if client != nil {
				client.Close()
			}
			if err == nil {
				t.Fatal("Connect should fail on malformed HELLO_ACK")
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("Connect error = %v, want %v", err, tc.want)
			}
			if tc.wantText != "" && !containsErrText(err, tc.wantText) {
				t.Fatalf("Connect error = %v, want text %q", err, tc.wantText)
			}
			if serr := <-serverDone; serr != nil {
				t.Fatalf("raw server failed: %v (path %s)", serr, path)
			}
		})
	}
}

func TestUdsClientHandshakePeerDisconnectBeforeAck(t *testing.T) {
	runDir := testRunDir(t)
	service := uniqueService(t)
	lfd, path := rawSeqpacketListener(t, runDir, service)

	serverDone := make(chan error, 1)
	go func() {
		nfd, _, err := syscall.Accept(lfd)
		if err != nil {
			serverDone <- err
			return
		}
		var helloBuf [128]byte
		_, recvErr := rawRecv(nfd, helloBuf[:])
		_ = syscall.Close(nfd)
		serverDone <- recvErr
	}()

	client, err := Connect(runDir, service, defaultClientConfigPtr())
	if client != nil {
		client.Close()
	}
	if err == nil {
		t.Fatal("Connect should fail when peer disconnects before HELLO_ACK")
	}
	if !errors.Is(err, ErrRecv) {
		t.Fatalf("Connect error = %v, want ErrRecv", err)
	}
	if serr := <-serverDone; serr != nil && !errors.Is(serr, ErrRecv) {
		t.Fatalf("raw server failed: %v (path %s)", serr, path)
	}
}

func TestUdsServerHandshakeRejectsMalformedHello(t *testing.T) {
	cases := []struct {
		name     string
		packetFn func() []byte
		want     error
		wantText string
	}{
		{
			name: "bad hello header",
			packetFn: func() []byte {
				hello := validHelloPacket()
				hello[0] = 0
				return hello
			},
			want:     ErrProtocol,
			wantText: "hello header",
		},
		{
			name: "wrong kind",
			packetFn: func() []byte {
				payload := make([]byte, helloPayloadSize)
				return encodePacket(protocol.Header{
					Kind:      protocol.KindRequest,
					Code:      protocol.CodeHello,
					ItemCount: 1,
				}, payload)
			},
			want:     ErrProtocol,
			wantText: "expected HELLO",
		},
		{
			name: "truncated payload",
			packetFn: func() []byte {
				hello := validHelloPacket()
				return hello[:protocol.HeaderSize+8]
			},
			want:     ErrProtocol,
			wantText: truncatedText("hello payload"),
		},
		{
			name: "bad hello payload layout",
			packetFn: func() []byte {
				hello := validHelloPacket()
				binary.NativeEndian.PutUint16(hello[protocol.HeaderSize:protocol.HeaderSize+2], 2)
				return hello
			},
			want:     ErrIncompatible,
			wantText: "protocol or layout version mismatch",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runDir := testRunDir(t)
			service := uniqueService(t)
			lfd, path := rawSeqpacketListener(t, runDir, service)

			handshakeDone := make(chan error, 1)
			go func() {
				nfd, _, err := syscall.Accept(lfd)
				if err != nil {
					handshakeDone <- err
					return
				}
				defer syscall.Close(nfd)

				_, err = serverHandshake(nfd, defaultServerConfigPtr(), 1)
				handshakeDone <- err
			}()

			cfd := rawSeqpacketConnect(t, path)
			if err := rawSendPacket(cfd, tc.packetFn()); err != nil {
				t.Fatalf("rawSendPacket failed: %v", err)
			}
			endStreamWrites(cfd)

			err := <-handshakeDone
			if err == nil {
				t.Fatal("serverHandshake should fail on malformed HELLO")
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("serverHandshake error = %v, want %v", err, tc.want)
			}
			if tc.wantText != "" && !containsErrText(err, tc.wantText) {
				t.Fatalf("serverHandshake error = %v, want text %q", err, tc.wantText)
			}
		})
	}
}

func TestUdsServerHandshakePeerDisconnectBeforeHello(t *testing.T) {
	runDir := testRunDir(t)
	service := uniqueService(t)
	lfd, path := rawSeqpacketListener(t, runDir, service)

	handshakeDone := make(chan error, 1)
	go func() {
		nfd, _, err := syscall.Accept(lfd)
		if err != nil {
			handshakeDone <- err
			return
		}
		defer syscall.Close(nfd)

		_, err = serverHandshake(nfd, defaultServerConfigPtr(), 1)
		handshakeDone <- err
	}()

	cfd := rawSeqpacketConnect(t, path)
	_ = syscall.Close(cfd)

	err := <-handshakeDone
	if err == nil {
		t.Fatal("serverHandshake should fail when peer disconnects before HELLO")
	}
	if !errors.Is(err, ErrRecv) {
		t.Fatalf("serverHandshake error = %v, want ErrRecv", err)
	}
}

func defaultClientConfigPtr() *ClientConfig {
	cfg := defaultClientConfig()
	return &cfg
}

func defaultServerConfigPtr() *ServerConfig {
	cfg := defaultServerConfig()
	return &cfg
}

// truncatedText is the error text for a packet shorter than its header
// declares: a SEQPACKET receiver sees the short packet and reports what it
// lacks, a stream receiver hits EOF inside the packet.
func truncatedText(seqpacket string) string {
	if streamTransport {
		return "truncated packet"
	}
	return seqpacket
}

func containsErrText(err error, want string) bool {
	return err != nil && want != "" && strings.Contains(err.Error(), want)
}

// A server between bind() and listen() refuses connects just like a dead
// server's leftover socket file. A second server starting in that window
// must not take the endpoint over: it fails with ErrAddrInUse, and the
// first server stays reachable.
func TestListenExcludesStartingServer(t *testing.T) {
	runDir := testRunDir(t)
	service := uniqueService(t)
	path := filepath.Join(runDir, service+".sock")
	_ = os.Remove(path)

	bound := make(chan struct{})
	release := make(chan struct{})
	hook := func(p string) {
		if p != path {
			return
		}
		close(bound)
		<-release
	}
	listenHook.Store(&hook)

	type result struct {
		l   *Listener
		err error
	}
	first := make(chan result, 1)
	go func() {
		l, err := Listen(runDir, service, defaultServerConfig())
		first <- result{l, err}
	}()
	select {
	case <-bound:
	case <-time.After(10 * time.Second):
		t.Fatal("first server never bound")
	}
	listenHook.Store(nil)

	second, err := Listen(runDir, service, defaultServerConfig())
	if !errors.Is(err, ErrAddrInUse) {
		if second != nil {
			second.Close()
		}
		close(release)
		<-first
		t.Fatalf("second server error = %v, want ErrAddrInUse", err)
	}

	close(release)
	r := <-first
	if r.err != nil {
		t.Fatalf("first listen: %v", r.err)
	}

	// A connect to the endpoint must reach the first server.
	cfd, err := syscall.Socket(syscall.AF_UNIX, sockType, 0)
	if err != nil {
		t.Fatalf("socket: %v", err)
	}
	defer syscall.Close(cfd)
	if err := syscall.Connect(cfd, &syscall.SockaddrUnix{Name: path}); err != nil {
		t.Fatalf("connect to endpoint: %v", err)
	}
	if err := pollReadableForReceive(r.l.Fd(), newReceiveWait(10000, nil)); err != nil {
		t.Fatalf("first server never received the connection: %v", err)
	}
	r.l.Close()

	// After the owner closed, the endpoint can be taken again.
	again, err := Listen(runDir, service, defaultServerConfig())
	if err != nil {
		t.Fatalf("endpoint not free after close: %v", err)
	}
	again.Close()
}

// A lock file left with a mode missing owner read or write (a server that
// ran under such a umask) must not block a start, and the start restores it.
func TestListenOverLeftoverLockModes(t *testing.T) {
	runDir := testRunDir(t)
	for _, mode := range []os.FileMode{0o200, 0o400} {
		service := uniqueService(t)
		lock := filepath.Join(runDir, service+".lock")
		_ = os.Remove(lock)
		if err := os.WriteFile(lock, nil, 0o600); err != nil {
			t.Fatalf("create lock: %v", err)
		}
		if err := os.Chmod(lock, mode); err != nil {
			t.Fatalf("chmod lock: %v", err)
		}

		first, err := Listen(runDir, service, defaultServerConfig())
		if err != nil {
			t.Fatalf("listen over a %o lock: %v", mode, err)
		}
		info, err := os.Stat(lock)
		if err != nil {
			t.Errorf("stat lock after listen over %o: %v", mode, err)
		} else if info.Mode().Perm() != 0o600 {
			t.Errorf("lock mode after listen over %o = %v, want 0600", mode, info.Mode().Perm())
		}
		if second, err := Listen(runDir, service, defaultServerConfig()); !errors.Is(err, ErrAddrInUse) {
			if second != nil {
				second.Close()
			}
			t.Errorf("second server over a %o lock: err = %v, want ErrAddrInUse", mode, err)
		}
		first.Close()
		_ = os.Remove(lock)
	}
}

// An empty run_dir is a caller error: Go would otherwise put the socket in
// the working directory and the lock at "/<name>.lock".
func TestEmptyRunDirRejected(t *testing.T) {
	if l, err := Listen("", "go_empty_run_dir", defaultServerConfig()); !errors.Is(err, ErrBadParam) {
		if l != nil {
			l.Close()
		}
		t.Errorf("Listen with empty run_dir: err = %v, want ErrBadParam", err)
	}
	if s, err := Connect("", "go_empty_run_dir", defaultClientConfigPtr()); !errors.Is(err, ErrBadParam) {
		if s != nil {
			s.Close()
		}
		t.Errorf("Connect with empty run_dir: err = %v, want ErrBadParam", err)
	}
}

const lockUmaskHelperEnv = "NETIPC_LOCK_UMASK_HELPER"

// The umask is process-wide, so the restrictive-umask case runs in a child
// test process instead of the parallel test runner.
func TestListenLockUnderRestrictiveUmask(t *testing.T) {
	testRunDir(t)
	for _, mask := range []string{"477", "277", "777"} {
		cmd := exec.Command(os.Args[0], "-test.run=^TestLockUmaskHelperProcess$", "-test.v")
		cmd.Env = append(os.Environ(), lockUmaskHelperEnv+"="+mask)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("endpoint lock under umask %s failed: %v\n%s", mask, err, out)
		}
	}
}

// TestLockUmaskHelperProcess is the child entry point; it is inert unless
// spawned by TestListenLockUnderRestrictiveUmask.
func TestLockUmaskHelperProcess(t *testing.T) {
	maskText := os.Getenv(lockUmaskHelperEnv)
	if maskText == "" {
		return
	}
	var mask int
	if _, err := fmt.Sscanf(maskText, "%o", &mask); err != nil {
		t.Fatalf("mask %q: %v", maskText, err)
	}
	runDir := testRunDir(t)
	service := "go_lock_umask_" + maskText
	lock := filepath.Join(runDir, service+".lock")
	_ = os.Remove(lock)
	_ = os.Remove(filepath.Join(runDir, service+".sock"))

	var published []string
	var tmpMode os.FileMode
	lockVisible := false
	hook := func(tmp string) {
		published = append(published, tmp)
		if info, err := os.Stat(tmp); err == nil {
			tmpMode = info.Mode().Perm()
		}
		_, err := os.Lstat(lock)
		lockVisible = err == nil
	}
	lockPublishHook.Store(&hook)

	// connect() needs write permission on the socket file. macOS sets the
	// socket to exactly 0600; Linux and FreeBSD keep the umask narrowing and
	// refuse to listen when it removes owner write.
	sock := filepath.Join(runDir, service+".sock")
	if runtime.GOOS != "darwin" && mask&0o200 != 0 {
		var err error
		func() {
			defer syscall.Umask(syscall.Umask(mask))
			var first *Listener
			first, err = Listen(runDir, service, defaultServerConfig())
			if first != nil {
				first.Close()
			}
		}()
		lockPublishHook.Store(nil)
		if !errors.Is(err, ErrSocket) || !strings.Contains(err.Error(), "permission denied") {
			t.Fatalf("listen under umask %o: err = %v, want a permission-denied socket error", mask, err)
		}
		if _, err := os.Lstat(sock); err == nil {
			t.Fatal("an unreachable socket was left behind")
		}
		again, err := Listen(runDir, service, defaultServerConfig())
		if err != nil {
			t.Fatalf("listen under a usable umask: %v", err)
		}
		again.Close()
	} else {
		lockUnderUmaskListens(t, runDir, service, sock, mask)
	}

	info, err := os.Stat(lock)
	if err != nil {
		t.Fatalf("lock stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("lock mode under umask %o = %o, want 600", mask, mode)
	}
	if len(published) != 1 || tmpMode != 0o600 || lockVisible {
		t.Fatalf("publish: names=%v mode=%o lock visible=%v, want one name, 600, false",
			published, tmpMode, lockVisible)
	}
	if _, err := os.Lstat(published[0]); err == nil {
		t.Errorf("temporary name %s was left behind", published[0])
	}
	_ = os.Remove(lock)
}

// lockUnderUmaskListens runs a listener under mask: it starts, a second one
// gets ErrAddrInUse, and it restarts; its socket has owner write only among
// the permission bits it keeps (exactly 0600 on macOS).
func lockUnderUmaskListens(t *testing.T, runDir, service, sock string, mask int) {
	t.Helper()
	// Restore the umask before exit so files the process writes then
	// (coverage data) keep their normal mode.
	defer syscall.Umask(syscall.Umask(mask))
	first, err := Listen(runDir, service, defaultServerConfig())
	lockPublishHook.Store(nil)
	if err != nil {
		t.Fatalf("listen under umask %o: %v", mask, err)
	}
	info, err := os.Lstat(sock)
	if err != nil {
		t.Fatalf("socket stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode&0o200 == 0 || mode&0o077 != 0 ||
		(runtime.GOOS == "darwin" && mode != 0o600) {
		t.Errorf("socket mode under umask %o = %o", mask, mode)
	}
	if second, err := Listen(runDir, service, defaultServerConfig()); !errors.Is(err, ErrAddrInUse) {
		if second != nil {
			second.Close()
		}
		t.Errorf("second server under umask %o: err = %v, want ErrAddrInUse", mask, err)
	}
	first.Close()
	again, err := Listen(runDir, service, defaultServerConfig())
	if err != nil {
		t.Fatalf("restart under umask %o: %v", mask, err)
	}
	again.Close()
}

// A child that shares the lock's open file description (as a child forked
// by another thread does until it execs) must not keep the endpoint once
// the listener closes.
func TestListenCloseReleasesLockHeldByChild(t *testing.T) {
	runDir := testRunDir(t)
	service := uniqueService(t)
	l, err := Listen(runDir, service, defaultServerConfig())
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	child := exec.Command("/bin/sh", "-c", "read x")
	child.ExtraFiles = []*os.File{l.lock}
	gate, err := child.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	if err := child.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	defer func() {
		_ = gate.Close()
		_ = child.Wait()
	}()

	l.Close()
	again, err := Listen(runDir, service, defaultServerConfig())
	if err != nil {
		t.Fatalf("endpoint not free while the child holds the lock fd: %v", err)
	}
	again.Close()
}

func TestListenSocketOwnerOnly(t *testing.T) {
	runDir := testRunDir(t)
	service := uniqueService(t)
	l, err := Listen(runDir, service, defaultServerConfig())
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	st, statErr := os.Stat(filepath.Join(runDir, service+".sock"))
	l.Close()
	if statErr != nil {
		t.Fatalf("stat socket: %v", statErr)
	}
	if mode := st.Mode().Perm(); mode != 0o600 {
		t.Errorf("listener socket mode = %o, want 600", mode)
	}
}

// A server whose client never sends HELLO reports the wait's own error,
// as C and Rust do, not a generic receive failure.
func TestServerHandshakeWaitKeepsTimeoutAndAbort(t *testing.T) {
	aborted := make(chan struct{})
	close(aborted)
	cases := []struct {
		name string
		wait receiveWait
		want error
	}{
		{"deadline", newReceiveWait(20, nil), ErrTimeout},
		{"abort", newReceiveWait(0, aborted), ErrAborted},
	}
	for _, tc := range cases {
		fds, err := syscall.Socketpair(syscall.AF_UNIX, sockType, 0)
		if err != nil {
			t.Fatalf("socketpair: %v", err)
		}
		_, err = serverHandshakeWait(fds[0], defaultServerConfigPtr(), 1, tc.wait, nil)
		syscall.Close(fds[0])
		syscall.Close(fds[1])
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: %v, want %v", tc.name, err, tc.want)
		}
	}
}

// A receive into a buffer too small for a header fails without reading a
// byte, so the message stays queued and the session stays usable.
func TestReceiveUndersizedBufferKeepsSession(t *testing.T) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, sockType, 0)
	if err != nil {
		t.Fatalf("socketpair: %v", err)
	}
	newSession := func(fd int) *Session {
		return &Session{
			fd:                     fd,
			role:                   RoleServer,
			PacketSize:             4096,
			MaxRequestPayloadBytes: 1024,
			MaxRequestBatchItems:   1,
		}
	}
	rx, tx := newSession(fds[0]), newSession(fds[1])
	defer rx.Close()
	defer tx.Close()

	payload := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	hdr := protocol.Header{Kind: protocol.KindRequest, Code: 1, ItemCount: 1, MessageID: 7}
	if err := tx.Send(&hdr, payload); err != nil {
		t.Fatalf("send: %v", err)
	}

	if _, _, err := rx.Receive(make([]byte, protocol.HeaderSize-1)); !errors.Is(err, ErrBadParam) {
		t.Fatalf("undersized buffer: expected ErrBadParam, got %v", err)
	}
	h, p, err := rx.ReceiveTimeout(make([]byte, 256), 1000, nil)
	if err != nil || h.MessageID != 7 || string(p) != string(payload) {
		t.Fatalf("request still received: hdr %+v payload %v err %v", h, p, err)
	}
}
