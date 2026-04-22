//go:build windows

package windows

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

func rawPipePair(t *testing.T) (syscall.Handle, syscall.Handle) {
	t.Helper()

	service := uniquePipeService(t)
	pipeName, err := BuildPipeName(testPipeRunDir, service)
	if err != nil {
		t.Fatalf("BuildPipeName failed: %v", err)
	}

	serverHandle, err := createPipeInstance(pipeName, defaultPipeBufSize, true)
	if err != nil {
		t.Fatalf("createPipeInstance failed: %v", err)
	}
	t.Cleanup(func() {
		if serverHandle != syscall.InvalidHandle && serverHandle != 0 {
			disconnectNamedPipe(serverHandle)
			syscall.CloseHandle(serverHandle)
		}
	})

	acceptCh := make(chan error, 1)
	go func() {
		acceptCh <- connectNamedPipe(serverHandle)
	}()

	clientHandle, err := syscall.CreateFile(
		&pipeName[0],
		_GENERIC_READ|_GENERIC_WRITE,
		0,
		nil,
		_OPEN_EXISTING,
		0,
		0,
	)
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	t.Cleanup(func() {
		if clientHandle != syscall.InvalidHandle && clientHandle != 0 {
			syscall.CloseHandle(clientHandle)
		}
	})

	mode := uint32(_PIPE_READMODE_MESSAGE)
	if err := setNamedPipeHandleState(clientHandle, &mode); err != nil {
		t.Fatalf("setNamedPipeHandleState failed: %v", err)
	}

	if err := <-acceptCh; err != nil && !errors.Is(err, syscall.Errno(_ERROR_PIPE_CONNECTED)) {
		t.Fatalf("connectNamedPipe failed: %v", err)
	}

	return clientHandle, serverHandle
}

func sessionPair(t *testing.T, sCfg ServerConfig, cCfg ClientConfig) (*Session, *Session) {
	t.Helper()

	service := uniquePipeService(t)
	listener := startListener(t, testPipeRunDir, service, sCfg)
	t.Cleanup(listener.Close)

	acceptCh := acceptAsync(listener)

	client, err := Connect(testPipeRunDir, service, &cCfg)
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
		AgreedPacketSize:              defaultPacketSize,
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
		PacketSize:              defaultPacketSize,
	}

	payload := make([]byte, helloPayloadSize)
	hello.Encode(payload)
	return encodePacket(protocol.Header{
		Kind:      protocol.KindControl,
		Code:      protocol.CodeHello,
		ItemCount: 1,
	}, payload)
}

func TestPipeConnectRejectsInvalidServiceName(t *testing.T) {
	if _, err := Connect(testPipeRunDir, "bad/name", defaultClientConfigPtr()); !errors.Is(err, ErrBadParam) {
		t.Fatalf("Connect(invalid service) = %v, want ErrBadParam", err)
	}
}

func TestPipeConnectRejectsPipeNameTooLong(t *testing.T) {
	service := strings.Repeat("a", maxPipeNameChars)
	if _, err := Connect(testPipeRunDir, service, defaultClientConfigPtr()); !errors.Is(err, ErrPipeName) {
		t.Fatalf("Connect(long service) = %v, want ErrPipeName", err)
	}
}

func TestPipeAcceptUnblocksWhenListenerClosed(t *testing.T) {
	service := uniquePipeService(t)
	listener := startListener(t, testPipeRunDir, service, defaultServerConfig())

	acceptCh := acceptAsync(listener)
	time.Sleep(50 * time.Millisecond)
	listener.Close()

	select {
	case sr := <-acceptCh:
		if sr.err == nil {
			if sr.session != nil {
				sr.session.Close()
			}
			t.Fatal("Accept after listener close should fail")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Accept did not unblock after listener close")
	}
}

func TestRawRecvMoreData(t *testing.T) {
	clientHandle, serverHandle := rawPipePair(t)

	msg := make([]byte, 96)
	for i := range msg {
		msg[i] = byte(i)
	}
	if err := rawWrite(clientHandle, msg); err != nil {
		t.Fatalf("rawWrite failed: %v", err)
	}

	small := make([]byte, 16)
	n, err := rawRecv(serverHandle, small)
	if err != nil {
		t.Fatalf("rawRecv(small) failed: %v", err)
	}
	if n != len(small) {
		t.Fatalf("rawRecv(small) len=%d, want %d", n, len(small))
	}

	rest := make([]byte, 96)
	n, err = rawRecv(serverHandle, rest)
	if err != nil {
		t.Fatalf("rawRecv(rest) failed: %v", err)
	}
	if n != len(msg)-len(small) {
		t.Fatalf("rawRecv(rest) len=%d, want %d", n, len(msg)-len(small))
	}
}

func TestRawPipeDisconnectErrors(t *testing.T) {
	clientHandle, serverHandle := rawPipePair(t)

	syscall.CloseHandle(serverHandle)
	serverHandle = syscall.InvalidHandle

	if err := rawWrite(clientHandle, []byte("x")); !errors.Is(err, ErrDisconnected) {
		t.Fatalf("rawWrite after peer close = %v, want ErrDisconnected", err)
	}

	clientHandle2, serverHandle2 := rawPipePair(t)
	syscall.CloseHandle(clientHandle2)
	clientHandle2 = syscall.InvalidHandle

	buf := make([]byte, 32)
	if _, err := rawRecv(serverHandle2, buf); !errors.Is(err, ErrDisconnected) {
		t.Fatalf("rawRecv after peer close = %v, want ErrDisconnected", err)
	}
}

func TestRawPipeGenericErrors(t *testing.T) {
	if err := rawWrite(syscall.InvalidHandle, []byte("x")); !errors.Is(err, ErrSend) {
		t.Fatalf("rawWrite(invalid handle) = %v, want ErrSend", err)
	}

	buf := make([]byte, 16)
	if _, err := rawRecv(syscall.InvalidHandle, buf); !errors.Is(err, ErrRecv) {
		t.Fatalf("rawRecv(invalid handle) = %v, want ErrRecv", err)
	}
}

func TestClientHandshakeRejectsMalformedAck(t *testing.T) {
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
			service := uniquePipeService(t)
			pipeName, err := BuildPipeName(testPipeRunDir, service)
			if err != nil {
				t.Fatalf("BuildPipeName failed: %v", err)
			}

			serverHandle, err := createPipeInstance(pipeName, defaultPipeBufSize, true)
			if err != nil {
				t.Fatalf("createPipeInstance failed: %v", err)
			}
			t.Cleanup(func() {
				if serverHandle != syscall.InvalidHandle && serverHandle != 0 {
					disconnectNamedPipe(serverHandle)
					syscall.CloseHandle(serverHandle)
				}
			})

			serverDone := make(chan error, 1)
			go func() {
				err := connectNamedPipe(serverHandle)
				if err != nil && !errors.Is(err, syscall.Errno(_ERROR_PIPE_CONNECTED)) {
					serverDone <- err
					return
				}

				var helloBuf [128]byte
				if _, err := rawRecv(serverHandle, helloBuf[:]); err != nil {
					serverDone <- err
					return
				}

				serverDone <- rawWrite(serverHandle, tc.packetFn())
			}()

			clientHandle, err := syscall.CreateFile(
				&pipeName[0],
				_GENERIC_READ|_GENERIC_WRITE,
				0,
				nil,
				_OPEN_EXISTING,
				0,
				0,
			)
			if err != nil {
				t.Fatalf("CreateFile failed: %v", err)
			}
			t.Cleanup(func() {
				if clientHandle != syscall.InvalidHandle && clientHandle != 0 {
					syscall.CloseHandle(clientHandle)
				}
			})

			mode := uint32(_PIPE_READMODE_MESSAGE)
			if err := setNamedPipeHandleState(clientHandle, &mode); err != nil {
				t.Fatalf("setNamedPipeHandleState failed: %v", err)
			}

			session, err := clientHandshake(clientHandle, defaultClientConfigPtr())
			if err == nil {
				session.Close()
				t.Fatal("clientHandshake should fail")
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("clientHandshake error = %v, want %v", err, tc.want)
			}
			if !strings.Contains(err.Error(), tc.wantText) {
				t.Fatalf("clientHandshake error = %v, want text %q", err, tc.wantText)
			}

			if err := <-serverDone; err != nil {
				t.Fatalf("raw server failed: %v", err)
			}
		})
	}
}

func TestClientHandshakeTransportErrors(t *testing.T) {
	t.Run("hello send disconnected", func(t *testing.T) {
		clientHandle, serverHandle := rawPipePair(t)

		if err := syscall.CloseHandle(serverHandle); err != nil {
			t.Fatalf("CloseHandle(server) failed: %v", err)
		}

		_, err := clientHandshake(clientHandle, defaultClientConfigPtr())
		if err == nil {
			t.Fatal("clientHandshake should fail")
		}
		if !errors.Is(err, ErrSend) {
			t.Fatalf("clientHandshake error = %v, want ErrSend", err)
		}
		if !strings.Contains(err.Error(), "hello send") {
			t.Fatalf("clientHandshake error = %v, want text %q", err, "hello send")
		}
	})

	t.Run("hello ack recv disconnected", func(t *testing.T) {
		clientHandle, serverHandle := rawPipePair(t)

		serverDone := make(chan error, 1)
		go func() {
			var helloBuf [128]byte
			_, err := rawRecv(serverHandle, helloBuf[:])
			if err == nil {
				err = syscall.CloseHandle(serverHandle)
			}
			serverDone <- err
		}()

		_, err := clientHandshake(clientHandle, defaultClientConfigPtr())
		if err == nil {
			t.Fatal("clientHandshake should fail")
		}
		if !errors.Is(err, ErrRecv) {
			t.Fatalf("clientHandshake error = %v, want ErrRecv", err)
		}
		if !strings.Contains(err.Error(), "hello_ack recv") {
			t.Fatalf("clientHandshake error = %v, want text %q", err, "hello_ack recv")
		}

		if err := <-serverDone; err != nil {
			t.Fatalf("raw server failed: %v", err)
		}
	})
}

func TestServerHandshakeRejectsMalformedHello(t *testing.T) {
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
			name: "wrong hello kind",
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
			clientHandle, serverHandle := rawPipePair(t)

			if err := rawWrite(clientHandle, tc.packetFn()); err != nil {
				t.Fatalf("rawWrite failed: %v", err)
			}

			cfg := defaultServerConfig()
			_, err := serverHandshake(serverHandle, &cfg, 1)
			if err == nil {
				t.Fatal("serverHandshake should fail")
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("serverHandshake error = %v, want %v", err, tc.want)
			}
			if !strings.Contains(err.Error(), tc.wantText) {
				t.Fatalf("serverHandshake error = %v, want text %q", err, tc.wantText)
			}
		})
	}
}

func TestServerHandshakeHelloAckSendFailure(t *testing.T) {
	clientHandle, serverHandle := rawPipePair(t)

	if err := rawWrite(clientHandle, validHelloPacket()); err != nil {
		t.Fatalf("rawWrite(hello) failed: %v", err)
	}
	if err := syscall.CloseHandle(clientHandle); err != nil {
		t.Fatalf("CloseHandle(client) failed: %v", err)
	}

	cfg := defaultServerConfig()
	_, err := serverHandshake(serverHandle, &cfg, 7)
	if err == nil {
		t.Fatal("serverHandshake should fail")
	}
	if !errors.Is(err, ErrSend) {
		t.Fatalf("serverHandshake error = %v, want ErrSend", err)
	}
	if !strings.Contains(err.Error(), "hello_ack send") {
		t.Fatalf("serverHandshake error = %v, want text %q", err, "hello_ack send")
	}
}

func TestPipeListenRejectsBadServiceNames(t *testing.T) {
	if _, err := Listen(testPipeRunDir, "bad/name", defaultServerConfig()); !errors.Is(err, ErrBadParam) {
		t.Fatalf("Listen(invalid service) = %v, want ErrBadParam", err)
	}

	service := strings.Repeat("a", maxPipeNameChars)
	if _, err := Listen(testPipeRunDir, service, defaultServerConfig()); !errors.Is(err, ErrPipeName) {
		t.Fatalf("Listen(long service) = %v, want ErrPipeName", err)
	}
}

func TestSessionReceiveRejectsMalformedMessages(t *testing.T) {
	cases := []struct {
		name     string
		sendFn   func(t *testing.T, sender syscall.Handle)
		want     error
		wantText string
	}{
		{
			name: "packet too short",
			sendFn: func(t *testing.T, sender syscall.Handle) {
				t.Helper()
				if err := rawWrite(sender, []byte{1, 2, 3, 4}); err != nil {
					t.Fatalf("rawWrite failed: %v", err)
				}
			},
			want:     ErrProtocol,
			wantText: "packet too short for header",
		},
		{
			name: "bad header",
			sendFn: func(t *testing.T, sender syscall.Handle) {
				t.Helper()
				hdr := protocol.Header{
					Magic:     0,
					Version:   protocol.Version,
					HeaderLen: protocol.HeaderLen,
					Kind:      protocol.KindRequest,
					Code:      protocol.MethodIncrement,
					ItemCount: 1,
				}
				pkt := make([]byte, protocol.HeaderSize)
				hdr.Encode(pkt)
				if err := rawWrite(sender, pkt); err != nil {
					t.Fatalf("rawWrite failed: %v", err)
				}
			},
			want:     ErrProtocol,
			wantText: "header decode",
		},
		{
			name: "payload exceeds negotiated max",
			sendFn: func(t *testing.T, sender syscall.Handle) {
				t.Helper()
				pkt := encodePacket(protocol.Header{
					Kind:       protocol.KindRequest,
					Code:       protocol.MethodIncrement,
					ItemCount:  1,
					PayloadLen: 0, // overwritten by encodePacket
				}, nil)
				hdr, err := protocol.DecodeHeader(pkt[:protocol.HeaderSize])
				if err != nil {
					t.Fatalf("DecodeHeader failed: %v", err)
				}
				hdr.PayloadLen = 5000
				hdr.Encode(pkt[:protocol.HeaderSize])
				if err := rawWrite(sender, pkt[:protocol.HeaderSize]); err != nil {
					t.Fatalf("rawWrite failed: %v", err)
				}
			},
			want:     ErrLimitExceeded,
			wantText: "payload_len",
		},
		{
			name: "item count exceeds negotiated max",
			sendFn: func(t *testing.T, sender syscall.Handle) {
				t.Helper()
				pkt := encodePacket(protocol.Header{
					Kind:      protocol.KindRequest,
					Code:      protocol.MethodIncrement,
					ItemCount: 99,
				}, nil)
				if err := rawWrite(sender, pkt); err != nil {
					t.Fatalf("rawWrite failed: %v", err)
				}
			},
			want:     ErrLimitExceeded,
			wantText: "item_count",
		},
		{
			name: "batch dir exceeds payload",
			sendFn: func(t *testing.T, sender syscall.Handle) {
				t.Helper()
				payload := make([]byte, 8)
				pkt := encodePacket(protocol.Header{
					Kind:      protocol.KindRequest,
					Code:      protocol.MethodIncrement,
					Flags:     protocol.FlagBatch,
					ItemCount: 2,
				}, payload)
				if err := rawWrite(sender, pkt); err != nil {
					t.Fatalf("rawWrite failed: %v", err)
				}
			},
			want:     ErrProtocol,
			wantText: "batch dir exceeds payload",
		},
		{
			name: "invalid batch dir",
			sendFn: func(t *testing.T, sender syscall.Handle) {
				t.Helper()
				payload := make([]byte, 24)
				binary.NativeEndian.PutUint32(payload[0:4], 1)
				binary.NativeEndian.PutUint32(payload[4:8], 4)
				binary.NativeEndian.PutUint32(payload[8:12], 0)
				binary.NativeEndian.PutUint32(payload[12:16], 4)
				copy(payload[16:], []byte("payload!!"))

				pkt := encodePacket(protocol.Header{
					Kind:      protocol.KindRequest,
					Code:      protocol.MethodIncrement,
					Flags:     protocol.FlagBatch,
					ItemCount: 2,
				}, payload)
				if err := rawWrite(sender, pkt); err != nil {
					t.Fatalf("rawWrite failed: %v", err)
				}
			},
			want:     ErrProtocol,
			wantText: "batch dir",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, server := sessionPair(t, defaultServerConfig(), defaultClientConfig())
			tc.sendFn(t, client.handle)

			buf := make([]byte, 4096)
			_, _, err := server.Receive(buf)
			if err == nil {
				t.Fatal("Receive should fail")
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("Receive error = %v, want %v", err, tc.want)
			}
			if !strings.Contains(err.Error(), tc.wantText) {
				t.Fatalf("Receive error = %v, want text %q", err, tc.wantText)
			}
		})
	}
}

func TestSessionReceiveRejectsMalformedChunks(t *testing.T) {
	type chunkSpec struct {
		totalPayloadLen uint32
		firstPayload    []byte
		chunkHeader     protocol.ChunkHeader
		chunkPayload    []byte
		closeSender     bool
	}

	cases := []struct {
		name     string
		spec     chunkSpec
		want     error
		wantText string
	}{
		{
			name: "bad chunk header",
			spec: chunkSpec{
				totalPayloadLen: 20,
				firstPayload:    []byte("0123456789"),
				chunkHeader: protocol.ChunkHeader{
					Magic:           protocol.MagicChunk,
					Version:         protocol.Version + 1,
					MessageID:       7,
					TotalMessageLen: uint32(protocol.HeaderSize + 20),
					ChunkIndex:      1,
					ChunkCount:      2,
					ChunkPayloadLen: 10,
				},
				chunkPayload: []byte("abcdefghij"),
			},
			want:     ErrChunk,
			wantText: "chunk header",
		},
		{
			name: "continuation recv disconnect",
			spec: chunkSpec{
				totalPayloadLen: 20,
				firstPayload:    []byte("0123456789"),
				closeSender:     true,
			},
			want:     ErrRecv,
			wantText: "continuation recv",
		},
		{
			name: "continuation too short",
			spec: chunkSpec{
				totalPayloadLen: 20,
				firstPayload:    []byte("0123456789"),
				chunkPayload:    []byte{1, 2, 3},
			},
			want:     ErrChunk,
			wantText: "continuation too short",
		},
		{
			name: "message id mismatch",
			spec: chunkSpec{
				totalPayloadLen: 20,
				firstPayload:    []byte("0123456789"),
				chunkHeader: protocol.ChunkHeader{
					Magic:           protocol.MagicChunk,
					Version:         protocol.Version,
					MessageID:       999,
					TotalMessageLen: uint32(protocol.HeaderSize + 20),
					ChunkIndex:      1,
					ChunkCount:      2,
					ChunkPayloadLen: 10,
				},
				chunkPayload: []byte("abcdefghij"),
			},
			want:     ErrChunk,
			wantText: "message_id mismatch",
		},
		{
			name: "chunk index mismatch",
			spec: chunkSpec{
				totalPayloadLen: 20,
				firstPayload:    []byte("0123456789"),
				chunkHeader: protocol.ChunkHeader{
					Magic:           protocol.MagicChunk,
					Version:         protocol.Version,
					MessageID:       7,
					TotalMessageLen: uint32(protocol.HeaderSize + 20),
					ChunkIndex:      2,
					ChunkCount:      2,
					ChunkPayloadLen: 10,
				},
				chunkPayload: []byte("abcdefghij"),
			},
			want:     ErrChunk,
			wantText: "chunk_index mismatch",
		},
		{
			name: "chunk count mismatch",
			spec: chunkSpec{
				totalPayloadLen: 20,
				firstPayload:    []byte("0123456789"),
				chunkHeader: protocol.ChunkHeader{
					Magic:           protocol.MagicChunk,
					Version:         protocol.Version,
					MessageID:       7,
					TotalMessageLen: uint32(protocol.HeaderSize + 20),
					ChunkIndex:      1,
					ChunkCount:      3,
					ChunkPayloadLen: 10,
				},
				chunkPayload: []byte("abcdefghij"),
			},
			want:     ErrChunk,
			wantText: "chunk_count mismatch",
		},
		{
			name: "total message len mismatch",
			spec: chunkSpec{
				totalPayloadLen: 20,
				firstPayload:    []byte("0123456789"),
				chunkHeader: protocol.ChunkHeader{
					Magic:           protocol.MagicChunk,
					Version:         protocol.Version,
					MessageID:       7,
					TotalMessageLen: uint32(protocol.HeaderSize + 21),
					ChunkIndex:      1,
					ChunkCount:      2,
					ChunkPayloadLen: 10,
				},
				chunkPayload: []byte("abcdefghij"),
			},
			want:     ErrChunk,
			wantText: "total_message_len mismatch",
		},
		{
			name: "chunk payload len mismatch",
			spec: chunkSpec{
				totalPayloadLen: 20,
				firstPayload:    []byte("0123456789"),
				chunkHeader: protocol.ChunkHeader{
					Magic:           protocol.MagicChunk,
					Version:         protocol.Version,
					MessageID:       7,
					TotalMessageLen: uint32(protocol.HeaderSize + 20),
					ChunkIndex:      1,
					ChunkCount:      2,
					ChunkPayloadLen: 9,
				},
				chunkPayload: []byte("abcdefghij"),
			},
			want:     ErrChunk,
			wantText: "chunk_payload_len mismatch",
		},
		{
			name: "chunk exceeds payload len",
			spec: chunkSpec{
				totalPayloadLen: 15,
				firstPayload:    []byte("0123456789"),
				chunkHeader: protocol.ChunkHeader{
					Magic:           protocol.MagicChunk,
					Version:         protocol.Version,
					MessageID:       7,
					TotalMessageLen: uint32(protocol.HeaderSize + 15),
					ChunkIndex:      1,
					ChunkCount:      2,
					ChunkPayloadLen: 10,
				},
				chunkPayload: []byte("abcdefghij"),
			},
			want:     ErrChunk,
			wantText: "chunk exceeds payload_len",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, server := sessionPair(t, defaultServerConfig(), defaultClientConfig())

			first := encodePacket(protocol.Header{
				Kind:      protocol.KindRequest,
				Code:      protocol.MethodIncrement,
				ItemCount: 1,
				MessageID: 7,
			}, tc.spec.firstPayload)
			hdr, err := protocol.DecodeHeader(first[:protocol.HeaderSize])
			if err != nil {
				t.Fatalf("DecodeHeader failed: %v", err)
			}
			hdr.PayloadLen = tc.spec.totalPayloadLen
			hdr.Encode(first[:protocol.HeaderSize])

			if err := rawWrite(client.handle, first); err != nil {
				t.Fatalf("rawWrite(first) failed: %v", err)
			}

			if tc.spec.closeSender {
				syscall.CloseHandle(client.handle)
				client.handle = syscall.InvalidHandle
			} else {
				var pkt []byte
				if len(tc.spec.chunkPayload) < protocol.HeaderSize && tc.spec.chunkHeader.Magic == 0 {
					pkt = tc.spec.chunkPayload
				} else {
					pkt = encodeChunkPacket(tc.spec.chunkHeader, tc.spec.chunkPayload)
				}
				if err := rawWrite(client.handle, pkt); err != nil {
					t.Fatalf("rawWrite(chunk) failed: %v", err)
				}
			}

			buf := make([]byte, 4096)
			_, _, err = server.Receive(buf)
			if err == nil {
				t.Fatal("Receive should fail")
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("Receive error = %v, want %v", err, tc.want)
			}
			if !strings.Contains(err.Error(), tc.wantText) {
				t.Fatalf("Receive error = %v, want text %q", err, tc.wantText)
			}
		})
	}
}

func TestSessionReceiveRejectsMalformedChunkedBatchMessages(t *testing.T) {
	type batchCase struct {
		name         string
		totalPayload uint32
		firstPayload []byte
		restPayload  []byte
		wantText     string
	}

	cases := []batchCase{
		{
			name:         "batch dir exceeds payload after chunking",
			totalPayload: 12,
			firstPayload: []byte("abcd"),
			restPayload:  []byte("efghijkl"),
			wantText:     "batch dir exceeds payload",
		},
		{
			name:         "invalid batch dir after chunking",
			totalPayload: 24,
			firstPayload: func() []byte {
				payload := make([]byte, 24)
				binary.NativeEndian.PutUint32(payload[0:4], 1)
				binary.NativeEndian.PutUint32(payload[4:8], 4)
				binary.NativeEndian.PutUint32(payload[8:12], 0)
				binary.NativeEndian.PutUint32(payload[12:16], 4)
				copy(payload[16:], []byte("payload!!"))
				return payload[:8]
			}(),
			restPayload: func() []byte {
				payload := make([]byte, 24)
				binary.NativeEndian.PutUint32(payload[0:4], 1)
				binary.NativeEndian.PutUint32(payload[4:8], 4)
				binary.NativeEndian.PutUint32(payload[8:12], 0)
				binary.NativeEndian.PutUint32(payload[12:16], 4)
				copy(payload[16:], []byte("payload!!"))
				return payload[8:]
			}(),
			wantText: "batch dir",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, server := sessionPair(t, defaultServerConfig(), defaultClientConfig())

			first := encodePacket(protocol.Header{
				Kind:      protocol.KindRequest,
				Code:      protocol.MethodIncrement,
				Flags:     protocol.FlagBatch,
				ItemCount: 2,
				MessageID: 7,
			}, tc.firstPayload)
			hdr, err := protocol.DecodeHeader(first[:protocol.HeaderSize])
			if err != nil {
				t.Fatalf("DecodeHeader failed: %v", err)
			}
			hdr.PayloadLen = tc.totalPayload
			hdr.Encode(first[:protocol.HeaderSize])

			chunk := encodeChunkPacket(protocol.ChunkHeader{
				Magic:           protocol.MagicChunk,
				Version:         protocol.Version,
				MessageID:       7,
				TotalMessageLen: uint32(protocol.HeaderSize) + tc.totalPayload,
				ChunkIndex:      1,
				ChunkCount:      2,
				ChunkPayloadLen: uint32(len(tc.restPayload)),
			}, tc.restPayload)

			if err := rawWrite(client.handle, first); err != nil {
				t.Fatalf("rawWrite(first) failed: %v", err)
			}
			if err := rawWrite(client.handle, chunk); err != nil {
				t.Fatalf("rawWrite(chunk) failed: %v", err)
			}

			buf := make([]byte, 4096)
			_, _, err = server.Receive(buf)
			if err == nil {
				t.Fatal("Receive should fail")
			}
			if !errors.Is(err, ErrProtocol) {
				t.Fatalf("Receive error = %v, want ErrProtocol", err)
			}
			if !strings.Contains(err.Error(), tc.wantText) {
				t.Fatalf("Receive error = %v, want text %q", err, tc.wantText)
			}
		})
	}
}

func TestSessionSendClearsInflightOnDisconnect(t *testing.T) {
	client, server := sessionPair(t, defaultServerConfig(), defaultClientConfig())
	server.Close()

	hdr := protocol.Header{
		Kind:      protocol.KindRequest,
		Code:      protocol.MethodIncrement,
		ItemCount: 1,
		MessageID: 77,
	}
	err := client.Send(&hdr, []byte("payload"))
	if err == nil {
		t.Fatal("Send should fail after peer disconnect")
	}
	if !errors.Is(err, ErrDisconnected) {
		t.Fatalf("Send error = %v, want ErrDisconnected", err)
	}
	if _, exists := client.inflightIDs[77]; exists {
		t.Fatalf("message_id 77 should be removed from inflightIDs after send failure")
	}
}

func TestSessionReceiveReportsPeerDisconnect(t *testing.T) {
	client, server := sessionPair(t, defaultServerConfig(), defaultClientConfig())
	server.Close()

	buf := make([]byte, 4096)
	_, _, err := client.Receive(buf)
	if err == nil {
		t.Fatal("Receive should fail after peer disconnect")
	}
	if !errors.Is(err, ErrDisconnected) {
		t.Fatalf("Receive error = %v, want ErrDisconnected", err)
	}
}

func TestSessionChunkedSendDisconnectPaths(t *testing.T) {
	t.Run("first chunk send failure", func(t *testing.T) {
		sCfg := defaultServerConfig()
		cCfg := defaultClientConfig()
		sCfg.PacketSize = 96
		cCfg.PacketSize = 96

		client, server := sessionPair(t, sCfg, cCfg)
		if err := syscall.CloseHandle(server.handle); err != nil {
			t.Fatalf("CloseHandle(server) failed: %v", err)
		}
		server.handle = syscall.InvalidHandle

		hdr := protocol.Header{
			Kind:      protocol.KindRequest,
			Code:      protocol.MethodIncrement,
			ItemCount: 1,
			MessageID: 88,
		}
		payload := bytes.Repeat([]byte("x"), 256)
		err := client.Send(&hdr, payload)
		if err == nil {
			t.Fatal("Send should fail after peer disconnect")
		}
		if !errors.Is(err, ErrDisconnected) {
			t.Fatalf("Send error = %v, want ErrDisconnected", err)
		}
		if _, exists := client.inflightIDs[88]; exists {
			t.Fatalf("message_id 88 should be removed from inflightIDs after chunked send failure")
		}
	})

	t.Run("continuation chunk send failure", func(t *testing.T) {
		sCfg := defaultServerConfig()
		cCfg := defaultClientConfig()
		sCfg.PacketSize = 96
		cCfg.PacketSize = 96

		client, server := sessionPair(t, sCfg, cCfg)

		closeDone := make(chan error, 1)
		go func() {
			buf := make([]byte, 96)
			_, err := rawRecv(server.handle, buf)
			if err == nil {
				err = syscall.CloseHandle(server.handle)
				server.handle = syscall.InvalidHandle
			}
			closeDone <- err
		}()

		hdr := protocol.Header{
			Kind:      protocol.KindRequest,
			Code:      protocol.MethodIncrement,
			ItemCount: 1,
			MessageID: 89,
		}
		payload := bytes.Repeat([]byte("y"), 4096)
		err := client.Send(&hdr, payload)
		if err == nil {
			buf := make([]byte, 96)
			_, _, err = client.Receive(buf)
			if err == nil {
				t.Fatal("session should observe disconnect after peer close")
			}
		}
		if !errors.Is(err, ErrDisconnected) && !errors.Is(err, ErrRecv) {
			t.Fatalf("error = %v, want ErrDisconnected or ErrRecv", err)
		}
		if _, exists := client.inflightIDs[89]; exists {
			t.Fatalf("message_id 89 should be removed from inflightIDs after session disconnect")
		}
		if err := <-closeDone; err != nil {
			t.Fatalf("server close goroutine failed: %v", err)
		}
	})
}

func TestSessionSendRejectsTooSmallPacketSize(t *testing.T) {
	sCfg := defaultServerConfig()
	cCfg := defaultClientConfig()
	sCfg.PacketSize = 16
	cCfg.PacketSize = 16

	service := uniquePipeService(t)
	listener := startListener(t, testPipeRunDir, service, sCfg)
	defer listener.Close()

	acceptCh := acceptAsync(listener)

	if _, err := Connect(testPipeRunDir, service, &cCfg); !errors.Is(err, ErrIncompatible) {
		t.Fatalf("expected ErrIncompatible, got %v", err)
	}

	sr := <-acceptCh
	if !errors.Is(sr.err, ErrIncompatible) {
		t.Fatalf("server expected ErrIncompatible, got %v", sr.err)
	}
}
