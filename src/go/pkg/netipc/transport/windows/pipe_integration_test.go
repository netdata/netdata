//go:build windows

package windows

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

const (
	testPipeRunDir = `C:\ProgramData\nipc_go_test`
	testAuthToken  = uint64(0xDEADBEEFCAFEBABE)
)

var testPipeCounter atomic.Uint64

func uniquePipeService(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("go_win_pipe_%d_%d", os.Getpid(), testPipeCounter.Add(1))
}

func defaultServerConfig() ServerConfig {
	return ServerConfig{
		SupportedProfiles:       protocol.ProfileBaseline,
		MaxRequestPayloadBytes:  4096,
		MaxRequestBatchItems:    16,
		MaxResponsePayloadBytes: 4096,
		MaxResponseBatchItems:   16,
		AuthToken:               testAuthToken,
	}
}

func defaultClientConfig() ClientConfig {
	return ClientConfig{
		SupportedProfiles:       protocol.ProfileBaseline,
		MaxRequestPayloadBytes:  4096,
		MaxRequestBatchItems:    16,
		MaxResponsePayloadBytes: 4096,
		MaxResponseBatchItems:   16,
		AuthToken:               testAuthToken,
	}
}

func defaultClientConfigPtr() *ClientConfig {
	cfg := defaultClientConfig()
	return &cfg
}

type serverResult struct {
	session *Session
	err     error
}

func startListener(t *testing.T, runDir, service string, cfg ServerConfig) *Listener {
	t.Helper()
	listener, err := Listen(runDir, service, cfg)
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	return listener
}

func acceptAsync(listener *Listener) <-chan serverResult {
	ch := make(chan serverResult, 1)
	go func() {
		session, err := listener.Accept()
		ch <- serverResult{session: session, err: err}
	}()
	return ch
}

func TestPipeSingleClientPingPong(t *testing.T) {
	service := uniquePipeService(t)
	listener := startListener(t, testPipeRunDir, service, defaultServerConfig())
	defer listener.Close()

	acceptCh := acceptAsync(listener)

	client, err := Connect(testPipeRunDir, service, defaultClientConfigPtr())
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	sr := <-acceptCh
	if sr.err != nil {
		t.Fatalf("Accept failed: %v", sr.err)
	}
	server := sr.session
	defer server.Close()

	payload := []byte("hello from client")
	hdr := protocol.Header{
		Kind:      protocol.KindRequest,
		Code:      protocol.MethodIncrement,
		ItemCount: 1,
		MessageID: 42,
	}
	if err := client.Send(&hdr, payload); err != nil {
		t.Fatalf("client Send: %v", err)
	}

	recvBuf := make([]byte, 4096)
	rHdr, rPayload, err := server.Receive(recvBuf)
	if err != nil {
		t.Fatalf("server Receive: %v", err)
	}
	if rHdr.Kind != protocol.KindRequest || rHdr.MessageID != 42 || !bytes.Equal(rPayload, payload) {
		t.Fatalf("unexpected server receive: hdr=%+v payload=%q", rHdr, rPayload)
	}

	respHdr := protocol.Header{
		Kind:      protocol.KindResponse,
		Code:      protocol.MethodIncrement,
		ItemCount: 1,
		MessageID: 42,
	}
	respPayload := []byte("response from server")
	if err := server.Send(&respHdr, respPayload); err != nil {
		t.Fatalf("server Send: %v", err)
	}

	rHdr, rPayload, err = client.Receive(recvBuf)
	if err != nil {
		t.Fatalf("client Receive: %v", err)
	}
	if rHdr.Kind != protocol.KindResponse || rHdr.MessageID != 42 || !bytes.Equal(rPayload, respPayload) {
		t.Fatalf("unexpected client receive: hdr=%+v payload=%q", rHdr, rPayload)
	}
}

func TestPipeMultiClient(t *testing.T) {
	service := uniquePipeService(t)
	listener := startListener(t, testPipeRunDir, service, defaultServerConfig())
	defer listener.Close()

	const numClients = 2
	clients := make([]*Session, numClients)
	servers := make([]*Session, numClients)

	for i := 0; i < numClients; i++ {
		acceptCh := acceptAsync(listener)
		c, err := Connect(testPipeRunDir, service, defaultClientConfigPtr())
		if err != nil {
			t.Fatalf("Connect[%d] failed: %v", i, err)
		}
		clients[i] = c

		sr := <-acceptCh
		if sr.err != nil {
			t.Fatalf("Accept[%d] failed: %v", i, sr.err)
		}
		servers[i] = sr.session
	}

	defer func() {
		for i := 0; i < numClients; i++ {
			clients[i].Close()
			servers[i].Close()
		}
	}()

	for i := 0; i < numClients; i++ {
		payload := []byte(fmt.Sprintf("client_%d", i))
		hdr := protocol.Header{
			Kind:      protocol.KindRequest,
			Code:      protocol.MethodIncrement,
			ItemCount: 1,
			MessageID: uint64(100 + i),
		}
		if err := clients[i].Send(&hdr, payload); err != nil {
			t.Fatalf("client[%d] Send: %v", i, err)
		}
	}

	buf := make([]byte, 4096)
	for i := 0; i < numClients; i++ {
		rHdr, rPayload, err := servers[i].Receive(buf)
		if err != nil {
			t.Fatalf("server[%d] Receive: %v", i, err)
		}
		expected := []byte(fmt.Sprintf("client_%d", i))
		if rHdr.MessageID != uint64(100+i) || !bytes.Equal(rPayload, expected) {
			t.Fatalf("server[%d] unexpected receive: hdr=%+v payload=%q", i, rHdr, rPayload)
		}

		resp := protocol.Header{
			Kind:      protocol.KindResponse,
			Code:      rHdr.Code,
			ItemCount: 1,
			MessageID: rHdr.MessageID,
		}
		if err := servers[i].Send(&resp, rPayload); err != nil {
			t.Fatalf("server[%d] Send: %v", i, err)
		}
	}

	for i := 0; i < numClients; i++ {
		rHdr, rPayload, err := clients[i].Receive(buf)
		if err != nil {
			t.Fatalf("client[%d] Receive: %v", i, err)
		}
		expected := []byte(fmt.Sprintf("client_%d", i))
		if rHdr.MessageID != uint64(100+i) || !bytes.Equal(rPayload, expected) {
			t.Fatalf("client[%d] unexpected receive: hdr=%+v payload=%q", i, rHdr, rPayload)
		}
	}
}

func TestPipePipelining(t *testing.T) {
	service := uniquePipeService(t)
	listener := startListener(t, testPipeRunDir, service, defaultServerConfig())
	defer listener.Close()

	acceptCh := acceptAsync(listener)

	client, err := Connect(testPipeRunDir, service, defaultClientConfigPtr())
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	sr := <-acceptCh
	if sr.err != nil {
		t.Fatalf("Accept failed: %v", sr.err)
	}
	server := sr.session
	defer server.Close()

	messageIDs := []uint64{10, 20, 30}
	for _, mid := range messageIDs {
		hdr := protocol.Header{
			Kind:      protocol.KindRequest,
			Code:      protocol.MethodIncrement,
			ItemCount: 1,
			MessageID: mid,
		}
		payload := []byte(fmt.Sprintf("req_%d", mid))
		if err := client.Send(&hdr, payload); err != nil {
			t.Fatalf("client Send(%d): %v", mid, err)
		}
	}

	buf := make([]byte, 4096)
	type reqInfo struct {
		hdr     protocol.Header
		payload []byte
	}
	reqs := make([]reqInfo, 0, len(messageIDs))
	for i := 0; i < len(messageIDs); i++ {
		rHdr, rPayload, err := server.Receive(buf)
		if err != nil {
			t.Fatalf("server Receive[%d]: %v", i, err)
		}
		reqs = append(reqs, reqInfo{hdr: rHdr, payload: append([]byte(nil), rPayload...)})
	}

	for i := len(reqs) - 1; i >= 0; i-- {
		resp := protocol.Header{
			Kind:      protocol.KindResponse,
			Code:      reqs[i].hdr.Code,
			ItemCount: 1,
			MessageID: reqs[i].hdr.MessageID,
		}
		respPayload := append([]byte("resp_"), reqs[i].payload...)
		if err := server.Send(&resp, respPayload); err != nil {
			t.Fatalf("server Send[%d]: %v", i, err)
		}
	}

	received := make(map[uint64][]byte)
	for i := 0; i < len(messageIDs); i++ {
		rHdr, rPayload, err := client.Receive(buf)
		if err != nil {
			t.Fatalf("client Receive[%d]: %v", i, err)
		}
		received[rHdr.MessageID] = append([]byte(nil), rPayload...)
	}

	for _, mid := range messageIDs {
		want := []byte(fmt.Sprintf("resp_req_%d", mid))
		if got, ok := received[mid]; !ok || !bytes.Equal(got, want) {
			t.Fatalf("missing or mismatched pipeline response for %d: %q", mid, got)
		}
	}
}

func TestPipeChunking(t *testing.T) {
	service := uniquePipeService(t)
	const forcedPacketSize = 128

	sCfg := defaultServerConfig()
	sCfg.PacketSize = forcedPacketSize
	sCfg.MaxRequestPayloadBytes = 65536
	sCfg.MaxResponsePayloadBytes = 65536
	listener := startListener(t, testPipeRunDir, service, sCfg)
	defer listener.Close()

	acceptCh := acceptAsync(listener)

	cCfg := defaultClientConfig()
	cCfg.PacketSize = forcedPacketSize
	cCfg.MaxRequestPayloadBytes = 65536
	cCfg.MaxResponsePayloadBytes = 65536
	client, err := Connect(testPipeRunDir, service, &cCfg)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	sr := <-acceptCh
	if sr.err != nil {
		t.Fatalf("Accept failed: %v", sr.err)
	}
	server := sr.session
	defer server.Close()

	if client.PacketSize > forcedPacketSize {
		t.Fatalf("client packet_size=%d, want <= %d", client.PacketSize, forcedPacketSize)
	}

	largePayload := make([]byte, 2000)
	for i := range largePayload {
		largePayload[i] = byte(i & 0xFF)
	}

	hdr := protocol.Header{
		Kind:      protocol.KindRequest,
		Code:      protocol.MethodIncrement,
		ItemCount: 1,
		MessageID: 99,
	}
	recvBuf := make([]byte, forcedPacketSize)
	type receiveResult struct {
		hdr     protocol.Header
		payload []byte
		err     error
	}
	serverRecvCh := make(chan receiveResult, 1)
	go func() {
		rHdr, rPayload, err := server.Receive(recvBuf)
		if err == nil {
			rPayload = append([]byte(nil), rPayload...)
		}
		serverRecvCh <- receiveResult{hdr: rHdr, payload: rPayload, err: err}
	}()

	if err := client.Send(&hdr, largePayload); err != nil {
		t.Fatalf("client Send (chunked): %v", err)
	}

	serverRecv := <-serverRecvCh
	if serverRecv.err != nil {
		t.Fatalf("server Receive (chunked): %v", serverRecv.err)
	}
	if serverRecv.hdr.MessageID != 99 || !bytes.Equal(serverRecv.payload, largePayload) {
		t.Fatalf("unexpected chunked receive: hdr=%+v payload-len=%d", serverRecv.hdr, len(serverRecv.payload))
	}

	resp := protocol.Header{
		Kind:      protocol.KindResponse,
		Code:      protocol.MethodIncrement,
		ItemCount: 1,
		MessageID: 99,
	}
	clientRecvCh := make(chan receiveResult, 1)
	go func() {
		rHdr, rPayload, err := client.Receive(recvBuf)
		if err == nil {
			rPayload = append([]byte(nil), rPayload...)
		}
		clientRecvCh <- receiveResult{hdr: rHdr, payload: rPayload, err: err}
	}()

	if err := server.Send(&resp, serverRecv.payload); err != nil {
		t.Fatalf("server Send (chunked echo): %v", err)
	}

	clientRecv := <-clientRecvCh
	if clientRecv.err != nil {
		t.Fatalf("client Receive (chunked): %v", clientRecv.err)
	}
	if clientRecv.hdr.MessageID != 99 || !bytes.Equal(clientRecv.payload, largePayload) {
		t.Fatalf("unexpected client chunked receive: hdr=%+v payload-len=%d", clientRecv.hdr, len(clientRecv.payload))
	}
}

func TestPipeHandshakeBadAuth(t *testing.T) {
	service := uniquePipeService(t)

	sCfg := defaultServerConfig()
	sCfg.AuthToken = 0x1111111111111111
	listener := startListener(t, testPipeRunDir, service, sCfg)
	defer listener.Close()

	acceptCh := acceptAsync(listener)

	cCfg := defaultClientConfig()
	cCfg.AuthToken = 0x2222222222222222
	if _, err := Connect(testPipeRunDir, service, &cCfg); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("expected ErrAuthFailed, got %v", err)
	}

	sr := <-acceptCh
	if !errors.Is(sr.err, ErrAuthFailed) {
		t.Fatalf("server expected ErrAuthFailed, got %v", sr.err)
	}
}

func TestPipeHandshakeProfileMismatch(t *testing.T) {
	service := uniquePipeService(t)

	sCfg := defaultServerConfig()
	sCfg.SupportedProfiles = protocol.ProfileSHMFutex
	listener := startListener(t, testPipeRunDir, service, sCfg)
	defer listener.Close()

	acceptCh := acceptAsync(listener)

	cCfg := defaultClientConfig()
	cCfg.SupportedProfiles = protocol.ProfileBaseline
	if _, err := Connect(testPipeRunDir, service, &cCfg); !errors.Is(err, ErrNoProfile) {
		t.Fatalf("expected ErrNoProfile, got %v", err)
	}

	sr := <-acceptCh
	if !errors.Is(sr.err, ErrNoProfile) {
		t.Fatalf("server expected ErrNoProfile, got %v", sr.err)
	}
}

func TestPipeHandshakeRequestPayloadOverCap(t *testing.T) {
	service := uniquePipeService(t)

	listener := startListener(t, testPipeRunDir, service, defaultServerConfig())
	defer listener.Close()

	acceptCh := acceptAsync(listener)

	cCfg := defaultClientConfig()
	cCfg.MaxRequestPayloadBytes = protocol.MaxPayloadCap + 1
	if _, err := Connect(testPipeRunDir, service, &cCfg); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("expected ErrLimitExceeded, got %v", err)
	}

	sr := <-acceptCh
	if !errors.Is(sr.err, ErrLimitExceeded) {
		t.Fatalf("server expected ErrLimitExceeded, got %v", sr.err)
	}
}

func TestPipeHandshakeIncompatibleClassifierHelpers(t *testing.T) {
	hdr := protocol.Header{
		Magic:     protocol.MagicMsg,
		Version:   protocol.Version + 1,
		HeaderLen: protocol.HeaderLen,
		Kind:      protocol.KindControl,
		Code:      protocol.CodeHello,
	}
	hdrBuf := make([]byte, protocol.HeaderSize)
	hdr.Encode(hdrBuf)
	if !headerVersionIncompatible(hdrBuf, protocol.CodeHello) {
		t.Fatal("headerVersionIncompatible should detect bad HELLO version")
	}
	if headerVersionIncompatible(hdrBuf, protocol.CodeHelloAck) {
		t.Fatal("headerVersionIncompatible should respect expected code")
	}

	hello := protocol.Hello{LayoutVersion: 2}
	helloBuf := make([]byte, 44)
	hello.Encode(helloBuf)
	if !helloLayoutIncompatible(helloBuf) {
		t.Fatal("helloLayoutIncompatible should detect bad layout_version")
	}

	ack := protocol.HelloAck{LayoutVersion: 2}
	ackBuf := make([]byte, 48)
	ack.Encode(ackBuf)
	if !helloAckLayoutIncompatible(ackBuf) {
		t.Fatal("helloAckLayoutIncompatible should detect bad layout_version")
	}
}

func TestPipeAddrInUse(t *testing.T) {
	service := uniquePipeService(t)
	listener := startListener(t, testPipeRunDir, service, defaultServerConfig())
	defer listener.Close()

	if _, err := Listen(testPipeRunDir, service, defaultServerConfig()); !errors.Is(err, ErrAddrInUse) {
		t.Fatalf("expected ErrAddrInUse, got %v", err)
	}
}

func TestPipeDirectionalLimitNegotiation(t *testing.T) {
	service := uniquePipeService(t)

	sCfg := defaultServerConfig()
	sCfg.MaxRequestPayloadBytes = 2048
	sCfg.MaxRequestBatchItems = 8
	sCfg.MaxResponsePayloadBytes = 8192
	sCfg.MaxResponseBatchItems = 32
	listener := startListener(t, testPipeRunDir, service, sCfg)
	defer listener.Close()

	acceptCh := acceptAsync(listener)

	cCfg := defaultClientConfig()
	cCfg.MaxRequestPayloadBytes = 4096
	cCfg.MaxRequestBatchItems = 16
	cCfg.MaxResponsePayloadBytes = 4096
	cCfg.MaxResponseBatchItems = 16
	client, err := Connect(testPipeRunDir, service, &cCfg)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	sr := <-acceptCh
	if sr.err != nil {
		t.Fatalf("Accept failed: %v", sr.err)
	}
	server := sr.session
	defer server.Close()

	if client.MaxRequestPayloadBytes != 4096 || client.MaxRequestBatchItems != 16 || client.MaxResponsePayloadBytes != 8192 || client.MaxResponseBatchItems != 16 {
		t.Fatalf("unexpected negotiated client limits: %+v", client)
	}
	if client.SessionID == 0 {
		t.Fatal("session id must be non-zero")
	}
	if server.MaxRequestPayloadBytes != client.MaxRequestPayloadBytes ||
		server.MaxRequestBatchItems != client.MaxRequestBatchItems ||
		server.MaxResponsePayloadBytes != client.MaxResponsePayloadBytes ||
		server.MaxResponseBatchItems != client.MaxResponseBatchItems ||
		server.SessionID != client.SessionID {
		t.Fatalf("server/client negotiation mismatch: server=%+v client=%+v", server, client)
	}
}

func TestPipeProfileSelection(t *testing.T) {
	service := uniquePipeService(t)

	sCfg := defaultServerConfig()
	sCfg.SupportedProfiles = protocol.ProfileBaseline | protocol.ProfileSHMHybrid | protocol.ProfileSHMFutex
	sCfg.PreferredProfiles = protocol.ProfileSHMFutex
	listener := startListener(t, testPipeRunDir, service, sCfg)
	defer listener.Close()

	acceptCh := acceptAsync(listener)

	cCfg := defaultClientConfig()
	cCfg.SupportedProfiles = protocol.ProfileBaseline | protocol.ProfileSHMHybrid | protocol.ProfileSHMFutex
	cCfg.PreferredProfiles = protocol.ProfileSHMFutex | protocol.ProfileSHMHybrid
	client, err := Connect(testPipeRunDir, service, &cCfg)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	sr := <-acceptCh
	if sr.err != nil {
		t.Fatalf("Accept failed: %v", sr.err)
	}
	defer sr.session.Close()

	if client.SelectedProfile != protocol.ProfileSHMFutex {
		t.Fatalf("selected profile = 0x%x, want SHMFutex", client.SelectedProfile)
	}
}

func TestPipeEmptyPayload(t *testing.T) {
	service := uniquePipeService(t)
	listener := startListener(t, testPipeRunDir, service, defaultServerConfig())
	defer listener.Close()

	acceptCh := acceptAsync(listener)
	client, err := Connect(testPipeRunDir, service, defaultClientConfigPtr())
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	sr := <-acceptCh
	if sr.err != nil {
		t.Fatalf("Accept failed: %v", sr.err)
	}
	server := sr.session
	defer server.Close()

	hdr := protocol.Header{
		Kind:      protocol.KindRequest,
		Code:      protocol.MethodIncrement,
		ItemCount: 1,
		MessageID: 1,
	}
	if err := client.Send(&hdr, nil); err != nil {
		t.Fatalf("Send empty payload: %v", err)
	}

	buf := make([]byte, 4096)
	rHdr, rPayload, err := server.Receive(buf)
	if err != nil {
		t.Fatalf("Receive empty payload: %v", err)
	}
	if rHdr.MessageID != 1 || len(rPayload) != 0 {
		t.Fatalf("unexpected empty payload receive: hdr=%+v len=%d", rHdr, len(rPayload))
	}
}

func TestPipeConcurrentSendReceive(t *testing.T) {
	service := uniquePipeService(t)

	sCfg := defaultServerConfig()
	sCfg.MaxRequestPayloadBytes = 65536
	sCfg.MaxResponsePayloadBytes = 65536
	listener := startListener(t, testPipeRunDir, service, sCfg)
	defer listener.Close()

	acceptCh := acceptAsync(listener)
	cCfg := defaultClientConfig()
	cCfg.MaxRequestPayloadBytes = 65536
	cCfg.MaxResponsePayloadBytes = 65536
	client, err := Connect(testPipeRunDir, service, &cCfg)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	sr := <-acceptCh
	if sr.err != nil {
		t.Fatalf("Accept failed: %v", sr.err)
	}
	server := sr.session
	defer server.Close()

	const numMessages = 20
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 65600)
		for i := 0; i < numMessages; i++ {
			rHdr, rPayload, err := server.Receive(buf)
			if err != nil {
				t.Errorf("server Receive[%d]: %v", i, err)
				return
			}
			resp := protocol.Header{
				Kind:      protocol.KindResponse,
				Code:      rHdr.Code,
				ItemCount: 1,
				MessageID: rHdr.MessageID,
			}
			if err := server.Send(&resp, rPayload); err != nil {
				t.Errorf("server Send[%d]: %v", i, err)
				return
			}
		}
	}()

	for i := 0; i < numMessages; i++ {
		payload := []byte(fmt.Sprintf("message_%d", i))
		hdr := protocol.Header{
			Kind:      protocol.KindRequest,
			Code:      protocol.MethodIncrement,
			ItemCount: 1,
			MessageID: uint64(i),
		}
		if err := client.Send(&hdr, payload); err != nil {
			t.Fatalf("client Send[%d]: %v", i, err)
		}
	}

	received := make(map[uint64]bool)
	buf := make([]byte, 65600)
	for i := 0; i < numMessages; i++ {
		rHdr, _, err := client.Receive(buf)
		if err != nil {
			t.Fatalf("client Receive[%d]: %v", i, err)
		}
		received[rHdr.MessageID] = true
	}

	wg.Wait()

	for i := 0; i < numMessages; i++ {
		if !received[uint64(i)] {
			t.Fatalf("missing response for message_id %d", i)
		}
	}
}

func TestPipeClosedSessionErrors(t *testing.T) {
	service := uniquePipeService(t)
	listener := startListener(t, testPipeRunDir, service, defaultServerConfig())
	defer listener.Close()

	acceptCh := acceptAsync(listener)
	client, err := Connect(testPipeRunDir, service, defaultClientConfigPtr())
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	sr := <-acceptCh
	if sr.err != nil {
		t.Fatalf("Accept failed: %v", sr.err)
	}
	sr.session.Close()
	client.Close()

	hdr := protocol.Header{
		Kind:      protocol.KindRequest,
		Code:      protocol.MethodIncrement,
		ItemCount: 1,
		MessageID: 1,
	}
	if err := client.Send(&hdr, []byte("x")); err == nil {
		t.Fatal("Send on closed session should fail")
	}

	buf := make([]byte, 4096)
	if _, _, err := client.Receive(buf); err == nil {
		t.Fatal("Receive on closed session should fail")
	}
}

func TestPipeListenerClose(t *testing.T) {
	service := uniquePipeService(t)
	listener := startListener(t, testPipeRunDir, service, defaultServerConfig())
	listener.Close()

	if _, err := Connect(testPipeRunDir, service, defaultClientConfigPtr()); err == nil {
		t.Fatal("Connect after listener close should fail")
	}
}

func TestPipeDefaultsApplied(t *testing.T) {
	service := uniquePipeService(t)

	sCfg := ServerConfig{AuthToken: testAuthToken}
	listener := startListener(t, testPipeRunDir, service, sCfg)
	defer listener.Close()

	acceptCh := acceptAsync(listener)

	cCfg := ClientConfig{AuthToken: testAuthToken}
	client, err := Connect(testPipeRunDir, service, &cCfg)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	sr := <-acceptCh
	if sr.err != nil {
		t.Fatalf("Accept failed: %v", sr.err)
	}
	defer sr.session.Close()

	if client.MaxRequestPayloadBytes != protocol.MaxPayloadDefault || client.MaxRequestBatchItems != 1 || client.MaxResponsePayloadBytes != protocol.MaxPayloadDefault || client.MaxResponseBatchItems != 1 {
		t.Fatalf("unexpected defaults: %+v", client)
	}
	if client.PacketSize != defaultPacketSize {
		t.Fatalf("packet_size=%d, want %d", client.PacketSize, defaultPacketSize)
	}
	if client.SelectedProfile != protocol.ProfileBaseline {
		t.Fatalf("selected profile = 0x%x, want baseline", client.SelectedProfile)
	}
}

func TestPipeHighestBit(t *testing.T) {
	tests := []struct {
		in   uint32
		want uint32
	}{
		{0, 0},
		{1, 1},
		{0x03, 0x02},
		{0x07, 0x04},
		{0x80000000, 0x80000000},
		{0xFF, 0x80},
	}
	for _, tc := range tests {
		if got := highestBit(tc.in); got != tc.want {
			t.Fatalf("highestBit(0x%x) = 0x%x, want 0x%x", tc.in, got, tc.want)
		}
	}
}

func TestPipeMultipleChunkedMessages(t *testing.T) {
	service := uniquePipeService(t)
	const forcedPacketSize = 96

	sCfg := defaultServerConfig()
	sCfg.PacketSize = forcedPacketSize
	sCfg.MaxRequestPayloadBytes = 65536
	sCfg.MaxResponsePayloadBytes = 65536
	listener := startListener(t, testPipeRunDir, service, sCfg)
	defer listener.Close()

	acceptCh := acceptAsync(listener)

	cCfg := defaultClientConfig()
	cCfg.PacketSize = forcedPacketSize
	cCfg.MaxRequestPayloadBytes = 65536
	cCfg.MaxResponsePayloadBytes = 65536
	client, err := Connect(testPipeRunDir, service, &cCfg)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	sr := <-acceptCh
	if sr.err != nil {
		t.Fatalf("Accept failed: %v", sr.err)
	}
	server := sr.session
	defer server.Close()

	type receiveResult struct {
		hdr     protocol.Header
		payload []byte
		err     error
	}
	serverRecvCh := make(chan receiveResult, 3)
	go func() {
		buf := make([]byte, forcedPacketSize)
		for i := 0; i < 3; i++ {
			rHdr, rPayload, err := server.Receive(buf)
			if err == nil {
				rPayload = append([]byte(nil), rPayload...)
			}
			serverRecvCh <- receiveResult{hdr: rHdr, payload: rPayload, err: err}
			if err != nil {
				return
			}
		}
	}()

	for i := 0; i < 3; i++ {
		size := 500 + i*200
		payload := make([]byte, size)
		for j := range payload {
			payload[j] = byte((i*31 + j) & 0xFF)
		}

		hdr := protocol.Header{
			Kind:      protocol.KindRequest,
			Code:      protocol.MethodIncrement,
			ItemCount: 1,
			MessageID: uint64(i + 1),
		}
		if err := client.Send(&hdr, payload); err != nil {
			t.Fatalf("Send[%d]: %v", i, err)
		}

		serverRecv := <-serverRecvCh
		if serverRecv.err != nil {
			t.Fatalf("Receive[%d]: %v", i, serverRecv.err)
		}
		if serverRecv.hdr.MessageID != uint64(i+1) || !bytes.Equal(serverRecv.payload, payload) {
			t.Fatalf("unexpected chunked message[%d]: hdr=%+v len=%d", i, serverRecv.hdr, len(serverRecv.payload))
		}
	}
}

func TestPipeConnectNoServer(t *testing.T) {
	service := uniquePipeService(t)
	if _, err := Connect(testPipeRunDir, service, defaultClientConfigPtr()); err == nil {
		t.Fatal("expected error connecting to missing server")
	}
}

func TestPipeAcceptWithDelayedConnect(t *testing.T) {
	service := uniquePipeService(t)
	listener := startListener(t, testPipeRunDir, service, defaultServerConfig())
	defer listener.Close()

	done := make(chan bool, 1)
	go func() {
		acceptCh := acceptAsync(listener)
		select {
		case sr := <-acceptCh:
			if sr.err != nil {
				done <- false
				return
			}
			sr.session.Close()
			done <- true
		case <-time.After(5 * time.Second):
			done <- false
		}
	}()

	time.Sleep(50 * time.Millisecond)
	client, err := Connect(testPipeRunDir, service, defaultClientConfigPtr())
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	client.Close()

	if ok := <-done; !ok {
		t.Fatal("accept did not complete successfully")
	}
}

func TestPipeDuplicateMessageID(t *testing.T) {
	service := uniquePipeService(t)
	listener := startListener(t, testPipeRunDir, service, defaultServerConfig())
	defer listener.Close()

	acceptCh := acceptAsync(listener)
	client, err := Connect(testPipeRunDir, service, defaultClientConfigPtr())
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	sr := <-acceptCh
	if sr.err != nil {
		t.Fatalf("Accept failed: %v", sr.err)
	}
	defer sr.session.Close()

	hdr := protocol.Header{
		Kind:      protocol.KindRequest,
		Code:      protocol.MethodIncrement,
		ItemCount: 1,
		MessageID: 42,
	}
	if err := client.Send(&hdr, []byte("hello")); err != nil {
		t.Fatalf("first Send: %v", err)
	}

	if err := client.Send(&hdr, []byte("hello")); !errors.Is(err, ErrDuplicateMsgID) {
		t.Fatalf("expected ErrDuplicateMsgID, got %v", err)
	}
}

func TestPipeUnknownMessageID(t *testing.T) {
	service := uniquePipeService(t)
	listener := startListener(t, testPipeRunDir, service, defaultServerConfig())
	defer listener.Close()

	acceptCh := acceptAsync(listener)
	client, err := Connect(testPipeRunDir, service, defaultClientConfigPtr())
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	sr := <-acceptCh
	if sr.err != nil {
		t.Fatalf("Accept failed: %v", sr.err)
	}
	server := sr.session
	defer server.Close()

	hdr := protocol.Header{
		Kind:      protocol.KindRequest,
		Code:      protocol.MethodIncrement,
		ItemCount: 1,
		MessageID: 10,
	}
	if err := client.Send(&hdr, []byte("x")); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	buf := make([]byte, 4096)
	rHdr, rPayload, err := server.Receive(buf)
	if err != nil {
		t.Fatalf("server Receive failed: %v", err)
	}

	resp := protocol.Header{
		Kind:      protocol.KindResponse,
		Code:      rHdr.Code,
		ItemCount: 1,
		MessageID: 999,
	}
	if err := server.Send(&resp, rPayload); err != nil {
		t.Fatalf("server Send failed: %v", err)
	}

	if _, _, err := client.Receive(buf); !errors.Is(err, ErrUnknownMsgID) {
		t.Fatalf("expected ErrUnknownMsgID, got %v", err)
	}
}

func TestPipeHandleAndRole(t *testing.T) {
	service := uniquePipeService(t)
	listener := startListener(t, testPipeRunDir, service, defaultServerConfig())
	defer listener.Close()

	acceptCh := acceptAsync(listener)
	client, err := Connect(testPipeRunDir, service, defaultClientConfigPtr())
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	sr := <-acceptCh
	if sr.err != nil {
		t.Fatalf("Accept failed: %v", sr.err)
	}
	server := sr.session
	defer server.Close()

	if listener.Handle() == syscall.InvalidHandle {
		t.Fatal("listener handle should be valid")
	}
	if client.Handle() == syscall.InvalidHandle || server.Handle() == syscall.InvalidHandle {
		t.Fatal("session handles should be valid")
	}
	if client.GetRole() != RoleClient || server.GetRole() != RoleServer {
		t.Fatalf("unexpected roles client=%d server=%d", client.GetRole(), server.GetRole())
	}
}

func TestPipePipelineChunked(t *testing.T) {
	service := uniquePipeService(t)
	const forcedPacketSize = 128

	sCfg := defaultServerConfig()
	sCfg.PacketSize = forcedPacketSize
	sCfg.MaxRequestPayloadBytes = 65536
	sCfg.MaxResponsePayloadBytes = 65536
	listener := startListener(t, testPipeRunDir, service, sCfg)
	defer listener.Close()

	acceptCh := acceptAsync(listener)

	cCfg := defaultClientConfig()
	cCfg.PacketSize = forcedPacketSize
	cCfg.MaxRequestPayloadBytes = 65536
	cCfg.MaxResponsePayloadBytes = 65536
	client, err := Connect(testPipeRunDir, service, &cCfg)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	sr := <-acceptCh
	if sr.err != nil {
		t.Fatalf("Accept failed: %v", sr.err)
	}
	server := sr.session
	defer server.Close()

	sizes := []int{200, 500, 300, 800, 150}
	count := len(sizes)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, forcedPacketSize)
		for i := 0; i < count; i++ {
			rHdr, rPayload, err := server.Receive(buf)
			if err != nil {
				t.Errorf("server Receive[%d]: %v", i, err)
				return
			}
			resp := protocol.Header{
				Kind:      protocol.KindResponse,
				Code:      rHdr.Code,
				ItemCount: 1,
				MessageID: rHdr.MessageID,
			}
			if err := server.Send(&resp, rPayload); err != nil {
				t.Errorf("server Send[%d]: %v", i, err)
				return
			}
		}
	}()

	for i, sz := range sizes {
		payload := make([]byte, sz)
		for j := range payload {
			payload[j] = byte((i + j) & 0xFF)
		}
		hdr := protocol.Header{
			Kind:      protocol.KindRequest,
			Code:      protocol.MethodIncrement,
			ItemCount: 1,
			MessageID: uint64(i + 1),
		}
		if err := client.Send(&hdr, payload); err != nil {
			t.Fatalf("client Send[%d]: %v", i, err)
		}
	}

	buf := make([]byte, forcedPacketSize)
	for i, sz := range sizes {
		rHdr, rPayload, err := client.Receive(buf)
		if err != nil {
			t.Fatalf("client Receive[%d]: %v", i, err)
		}
		if rHdr.MessageID != uint64(i+1) {
			t.Errorf("[%d] message_id = %d, want %d", i, rHdr.MessageID, i+1)
		}
		if len(rPayload) != sz {
			t.Errorf("[%d] payload len = %d, want %d", i, len(rPayload), sz)
			continue
		}
		expected := make([]byte, sz)
		for j := range expected {
			expected[j] = byte((i + j) & 0xFF)
		}
		if !bytes.Equal(rPayload, expected) {
			t.Errorf("[%d] chunked payload data mismatch", i)
		}
	}

	wg.Wait()
}

func TestPipeBatchRoundTrip(t *testing.T) {
	service := uniquePipeService(t)
	listener := startListener(t, testPipeRunDir, service, defaultServerConfig())
	defer listener.Close()

	acceptCh := acceptAsync(listener)
	client, err := Connect(testPipeRunDir, service, defaultClientConfigPtr())
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	sr := <-acceptCh
	if sr.err != nil {
		t.Fatalf("Accept failed: %v", sr.err)
	}
	server := sr.session
	defer server.Close()

	payloadBuf := make([]byte, 256)
	builder := protocol.NewBatchBuilder(payloadBuf, 3)
	for _, v := range []uint64{1, 41, 99} {
		var item [protocol.IncrementPayloadSize]byte
		if protocol.IncrementEncode(v, item[:]) == 0 {
			t.Fatal("increment encode failed")
		}
		if err := builder.Add(item[:]); err != nil {
			t.Fatalf("batch add failed: %v", err)
		}
	}
	totalLen, _ := builder.Finish()
	reqPayload := payloadBuf[:totalLen]

	hdr := protocol.Header{
		Kind:      protocol.KindRequest,
		Code:      protocol.MethodIncrement,
		Flags:     protocol.FlagBatch,
		ItemCount: 3,
		MessageID: 7,
	}
	if err := client.Send(&hdr, reqPayload); err != nil {
		t.Fatalf("client Send batch failed: %v", err)
	}

	buf := make([]byte, 4096)
	rHdr, rPayload, err := server.Receive(buf)
	if err != nil {
		t.Fatalf("server Receive batch failed: %v", err)
	}
	if rHdr.Flags&protocol.FlagBatch == 0 || rHdr.ItemCount != 3 {
		t.Fatalf("unexpected batch header: %+v", rHdr)
	}
	for i, want := range []uint64{1, 41, 99} {
		item, err := protocol.BatchItemGet(rPayload, 3, uint32(i))
		if err != nil {
			t.Fatalf("BatchItemGet[%d] failed: %v", i, err)
		}
		got, err := protocol.IncrementDecode(item)
		if err != nil {
			t.Fatalf("IncrementDecode[%d] failed: %v", i, err)
		}
		if got != want {
			t.Fatalf("item[%d]=%d, want %d", i, got, want)
		}
	}
}

func TestPipeLargePayloadsMixedSizes(t *testing.T) {
	service := uniquePipeService(t)

	sCfg := defaultServerConfig()
	sCfg.MaxRequestPayloadBytes = 65536
	sCfg.MaxResponsePayloadBytes = 65536
	listener := startListener(t, testPipeRunDir, service, sCfg)
	defer listener.Close()

	acceptCh := acceptAsync(listener)

	cCfg := defaultClientConfig()
	cCfg.MaxRequestPayloadBytes = 65536
	cCfg.MaxResponsePayloadBytes = 65536
	client, err := Connect(testPipeRunDir, service, &cCfg)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	sr := <-acceptCh
	if sr.err != nil {
		t.Fatalf("Accept failed: %v", sr.err)
	}
	server := sr.session
	defer server.Close()

	sizes := []int{8, 256, 1024, 8, 256, 1024}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 8192)
		for i := 0; i < len(sizes); i++ {
			rHdr, rPayload, err := server.Receive(buf)
			if err != nil {
				t.Errorf("server Receive[%d]: %v", i, err)
				return
			}
			resp := protocol.Header{
				Kind:      protocol.KindResponse,
				Code:      rHdr.Code,
				ItemCount: 1,
				MessageID: rHdr.MessageID,
			}
			if err := server.Send(&resp, rPayload); err != nil {
				t.Errorf("server Send[%d]: %v", i, err)
				return
			}
		}
	}()

	for i, sz := range sizes {
		payload := make([]byte, sz)
		for j := range payload {
			payload[j] = byte((i*37 + j) & 0xFF)
		}
		hdr := protocol.Header{
			Kind:      protocol.KindRequest,
			Code:      protocol.MethodIncrement,
			ItemCount: 1,
			MessageID: uint64(i + 1),
		}
		if err := client.Send(&hdr, payload); err != nil {
			t.Fatalf("client Send[%d]: %v", i, err)
		}
	}

	buf := make([]byte, 8192)
	for i, sz := range sizes {
		rHdr, rPayload, err := client.Receive(buf)
		if err != nil {
			t.Fatalf("client Receive[%d]: %v", i, err)
		}
		if rHdr.MessageID != uint64(i+1) || len(rPayload) != sz {
			t.Fatalf("unexpected mixed-size response[%d]: hdr=%+v len=%d", i, rHdr, len(rPayload))
		}
		expected := make([]byte, sz)
		for j := range expected {
			expected[j] = byte((i*37 + j) & 0xFF)
		}
		if !bytes.Equal(rPayload, expected) {
			t.Fatalf("payload mismatch for mixed-size response[%d]", i)
		}
	}

	wg.Wait()
}

func TestPipeBinaryPayloadRoundTrip(t *testing.T) {
	service := uniquePipeService(t)
	listener := startListener(t, testPipeRunDir, service, defaultServerConfig())
	defer listener.Close()

	acceptCh := acceptAsync(listener)
	client, err := Connect(testPipeRunDir, service, defaultClientConfigPtr())
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	sr := <-acceptCh
	if sr.err != nil {
		t.Fatalf("Accept failed: %v", sr.err)
	}
	server := sr.session
	defer server.Close()

	payload := make([]byte, 8)
	binary.NativeEndian.PutUint64(payload, 0x1122334455667788)
	hdr := protocol.Header{
		Kind:      protocol.KindRequest,
		Code:      protocol.MethodIncrement,
		ItemCount: 1,
		MessageID: 55,
	}
	if err := client.Send(&hdr, payload); err != nil {
		t.Fatalf("client Send failed: %v", err)
	}

	buf := make([]byte, 4096)
	rHdr, rPayload, err := server.Receive(buf)
	if err != nil {
		t.Fatalf("server Receive failed: %v", err)
	}
	if rHdr.MessageID != 55 {
		t.Fatalf("unexpected message_id %d", rHdr.MessageID)
	}
	if got := binary.NativeEndian.Uint64(rPayload); got != 0x1122334455667788 {
		t.Fatalf("unexpected payload value 0x%x", got)
	}
}
