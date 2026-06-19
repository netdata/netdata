//go:build unix

package posix

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

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

	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_SEQPACKET, 0)
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

	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_SEQPACKET, 0)
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
	runDir := filepath.Join(t.TempDir(), "missing")
	_, err := Listen(runDir, uniqueService(t), defaultServerConfig())
	if !errors.Is(err, ErrSocket) {
		t.Fatalf("Listen(missing runDir) = %v, want ErrSocket", err)
	}
	if !containsErrText(err, "bind:") {
		t.Fatalf("Listen(missing runDir) = %v, want bind failure detail", err)
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

	expired := receiveWait{deadline: time.Now().Add(-time.Millisecond)}
	if _, err := expired.waitMs(); !errors.Is(err, ErrTimeout) {
		t.Fatalf("waitMs expired = %v, want ErrTimeout", err)
	}

	infinite := newReceiveWait(0, nil)
	if waitMs, err := infinite.waitMs(); err != nil || waitMs != receiveAbortPollMs {
		t.Fatalf("waitMs infinite = %d/%v, want %d/nil", waitMs, err, receiveAbortPollMs)
	}

	short := receiveWait{deadline: time.Now().Add(1500 * time.Microsecond)}
	if waitMs, err := short.waitMs(); err != nil || waitMs < 1 || waitMs > 2 {
		t.Fatalf("waitMs short deadline = %d/%v, want 1..2/nil", waitMs, err)
	}
}

func TestReceiveAndStaleDirectGuards(t *testing.T) {
	if err := pollReadableForReceive(1<<31, newReceiveWait(1, nil)); !errors.Is(err, ErrBadParam) {
		t.Fatalf("poll high fd error = %v, want ErrBadParam", err)
	}
	if _, err := rawRecvWithTimeout(1<<31, make([]byte, 1), newReceiveWait(1, nil)); !errors.Is(err, ErrBadParam) {
		t.Fatalf("recv high fd error = %v, want ErrBadParam", err)
	}

	closedFD, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_SEQPACKET, 0)
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
			first: 8,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, server := udsSessionPair(t, defaultServerConfig(), defaultClientConfig())

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

			buf := make([]byte, 4096)
			_, _, err := client.Receive(buf)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Receive error = %v, want %v", err, tc.want)
			}
		})
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
			wantText: "ack payload truncated",
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
			wantText: "hello payload",
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

func containsErrText(err error, want string) bool {
	return err != nil && want != "" && strings.Contains(err.Error(), want)
}

func TestDetectPacketSizeFallbackAndSuccess(t *testing.T) {
	if got := detectPacketSize(-1); got != defaultPacketSizeFallback {
		t.Fatalf("detectPacketSize(-1) = %d, want fallback %d", got, defaultPacketSizeFallback)
	}

	client, _ := udsSessionPair(t, defaultServerConfig(), defaultClientConfig())
	if got := detectPacketSize(client.fd); got == 0 {
		t.Fatal("detectPacketSize(valid fd) returned 0")
	}
}
