//go:build windows

package raw

import (
	"fmt"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	windows "github.com/netdata/netdata/go/plugins/pkg/netipc/transport/windows"
)

const (
	rawPipeAccessDuplex   = 0x00000003
	rawPipeTypeMessage    = 0x00000004
	rawPipeReadModeMsg    = 0x00000002
	rawPipeWait           = 0x00000000
	rawErrorPipeConnected = 535
	rawPipeBufSize        = 65536
)

var (
	rawKernel32         = syscall.NewLazyDLL("kernel32.dll")
	rawCreateNamedPipeW = rawKernel32.NewProc("CreateNamedPipeW")
	rawConnectNamedPipe = rawKernel32.NewProc("ConnectNamedPipe")
)

var winServiceCounter atomic.Uint64

func uniqueWinService(prefix string) string {
	return fmt.Sprintf("%s_%d_%d", prefix, os.Getpid(), winServiceCounter.Add(1))
}

func testWinShmServerConfig() windows.ServerConfig {
	cfg := testWinServerConfig()
	cfg.SupportedProfiles = protocol.ProfileBaseline | protocol.ProfileSHMHybrid
	cfg.PreferredProfiles = protocol.ProfileSHMHybrid
	return cfg
}

func testWinShmClientConfig() windows.ClientConfig {
	cfg := testWinClientConfig()
	cfg.SupportedProfiles = protocol.ProfileBaseline | protocol.ProfileSHMHybrid
	cfg.PreferredProfiles = protocol.ProfileSHMHybrid
	return cfg
}

func waitWinClientReady(t *testing.T, client *Client) {
	t.Helper()

	for i := 0; i < 100; i++ {
		client.Refresh()
		if client.Ready() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("client not ready, final state=%d", client.state)
}

func startTestServerWinWithConfig(
	service string,
	cfg windows.ServerConfig,
	expectedMethodCode uint16,
	handler DispatchHandler,
) *winTestServer {
	s := NewServer(winTestRunDir, service, cfg, expectedMethodCode, handler)
	doneCh := make(chan struct{})

	go func() {
		defer close(doneCh)
		_ = s.Run()
	}()

	time.Sleep(200 * time.Millisecond)

	return &winTestServer{server: s, doneCh: doneCh}
}

func startTestIncrementServerWinWithConfig(service string, cfg windows.ServerConfig) *winTestServer {
	return startTestServerWinWithConfig(service, cfg, protocol.MethodIncrement, winIncrementDispatchHandler())
}

func startTestStringReverseServerWinWithConfig(service string, cfg windows.ServerConfig) *winTestServer {
	return startTestServerWinWithConfig(service, cfg, protocol.MethodStringReverse, winStringReverseDispatchHandler())
}

func startTestSnapshotServerWinWithConfig(service string, cfg windows.ServerConfig) *winTestServer {
	return startTestServerWinWithConfig(service, cfg, protocol.MethodCgroupsSnapshot, winSnapshotDispatchHandler())
}

func newRawWinShmClient(t *testing.T, profile uint32) (*Client, *windows.WinShmContext) {
	t.Helper()

	runDir := t.TempDir()
	service := uniqueWinService("go_win_raw_shm")
	sessionID := winServiceCounter.Add(1)
	reqCap := uint32(protocol.HeaderSize + 4096)
	respCap := uint32(protocol.HeaderSize + winResponseBufSize)

	server, err := windows.WinShmServerCreate(runDir, service, winAuthToken, sessionID, profile, reqCap, respCap)
	if err != nil {
		t.Fatalf("WinShmServerCreate failed: %v", err)
	}

	clientShm, err := windows.WinShmClientAttach(runDir, service, winAuthToken, sessionID, profile)
	if err != nil {
		server.WinShmDestroy()
		t.Fatalf("WinShmClientAttach failed: %v", err)
	}

	cfg := testWinShmClientConfig()
	client := NewIncrementClient(runDir, service, cfg)
	client.state = StateReady
	client.shm = clientShm

	t.Cleanup(func() {
		client.Close()
		server.WinShmDestroy()
	})

	return client, server
}

func encodeRawWinMessage(hdr protocol.Header, payload []byte) []byte {
	hdr.Magic = protocol.MagicMsg
	hdr.Version = protocol.Version
	hdr.HeaderLen = protocol.HeaderLen
	hdr.PayloadLen = uint32(len(payload))

	msg := make([]byte, protocol.HeaderSize+len(payload))
	hdr.Encode(msg[:protocol.HeaderSize])
	copy(msg[protocol.HeaderSize:], payload)
	return msg
}

func encodeWinIncrementBatchPayload(t *testing.T, values ...uint64) []byte {
	t.Helper()

	itemCount := uint32(len(values))
	if itemCount == 0 {
		return nil
	}

	bufSize := protocol.Align8(int(itemCount)*8) +
		int(itemCount)*protocol.IncrementPayloadSize +
		int(itemCount)*protocol.Alignment
	buf := make([]byte, bufSize)
	bb := protocol.NewBatchBuilder(buf, itemCount)

	for _, v := range values {
		var item [protocol.IncrementPayloadSize]byte
		if protocol.IncrementEncode(v, item[:]) == 0 {
			t.Fatal("IncrementEncode failed")
		}
		if err := bb.Add(item[:]); err != nil {
			t.Fatalf("batch add failed: %v", err)
		}
	}

	n, _ := bb.Finish()
	return buf[:n]
}

type winRawSessionServer struct {
	listener *windows.Listener
	doneCh   chan error
}

type winHelloAckServer struct {
	doneCh   chan error
	accepted atomic.Uint32
}

func encodeHelloAckPacketWithVersion(version uint16, status uint16, layoutVersion uint16) []byte {
	ack := protocol.HelloAck{
		LayoutVersion:                 layoutVersion,
		Flags:                         0,
		ServerSupportedProfiles:       protocol.ProfileBaseline,
		IntersectionProfiles:          protocol.ProfileBaseline,
		SelectedProfile:               protocol.ProfileBaseline,
		AgreedMaxRequestPayloadBytes:  protocol.MaxPayloadDefault,
		AgreedMaxRequestBatchItems:    1,
		AgreedMaxResponsePayloadBytes: winResponseBufSize,
		AgreedMaxResponseBatchItems:   1,
		AgreedPacketSize:              0,
		SessionID:                     77,
	}

	payload := make([]byte, 48)
	n := ack.Encode(payload)
	payload = payload[:n]

	hdr := protocol.Header{
		Magic:           protocol.MagicMsg,
		Version:         version,
		HeaderLen:       protocol.HeaderSize,
		Kind:            protocol.KindControl,
		Code:            protocol.CodeHelloAck,
		TransportStatus: status,
		PayloadLen:      uint32(len(payload)),
		ItemCount:       1,
		MessageID:       0,
	}

	pkt := make([]byte, protocol.HeaderSize+len(payload))
	hdr.Encode(pkt[:protocol.HeaderSize])
	copy(pkt[protocol.HeaderSize:], payload)
	return pkt
}

func startRawWinHelloAckServer(t *testing.T, service string, packet []byte) *winHelloAckServer {
	t.Helper()

	if err := os.MkdirAll(winTestRunDir, 0o700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	pipeName, err := windows.BuildPipeName(winTestRunDir, service)
	if err != nil {
		t.Fatalf("BuildPipeName failed: %v", err)
	}

	pipe, err := createRawNamedPipe(
		&pipeName[0],
		rawPipeAccessDuplex,
		rawPipeTypeMessage|rawPipeReadModeMsg|rawPipeWait,
		1,
		rawPipeBufSize,
		rawPipeBufSize,
		0,
	)
	if err != nil {
		t.Fatalf("CreateNamedPipe failed: %v", err)
	}

	srv := &winHelloAckServer{
		doneCh: make(chan error, 1),
	}

	go func() {
		defer close(srv.doneCh)
		defer syscall.CloseHandle(pipe)

		err := connectRawNamedPipe(pipe)
		if err != nil && err != syscall.Errno(rawErrorPipeConnected) {
			srv.doneCh <- fmt.Errorf("ConnectNamedPipe failed: %w", err)
			return
		}

		srv.accepted.Store(1)

		helloBuf := make([]byte, 256)
		var helloN uint32
		if err := syscall.ReadFile(pipe, helloBuf, &helloN, nil); err != nil {
			srv.doneCh <- fmt.Errorf("ReadFile failed: %w", err)
			return
		}
		if helloN == 0 {
			srv.doneCh <- fmt.Errorf("ReadFile returned zero bytes")
			return
		}

		var written uint32
		if err := syscall.WriteFile(pipe, packet, &written, nil); err != nil {
			srv.doneCh <- fmt.Errorf("WriteFile failed: %w", err)
			return
		}
		if int(written) != len(packet) {
			srv.doneCh <- fmt.Errorf("short write: %d/%d", written, len(packet))
			return
		}

		srv.doneCh <- nil
	}()

	time.Sleep(200 * time.Millisecond)
	return srv
}

func createRawNamedPipe(name *uint16, openMode uint32, pipeMode uint32, maxInstances uint32,
	outBufferSize uint32, inBufferSize uint32, defaultTimeout uint32) (syscall.Handle, error) {
	r0, _, e1 := rawCreateNamedPipeW.Call(
		uintptr(unsafe.Pointer(name)),
		uintptr(openMode),
		uintptr(pipeMode),
		uintptr(maxInstances),
		uintptr(outBufferSize),
		uintptr(inBufferSize),
		uintptr(defaultTimeout),
		0,
	)
	handle := syscall.Handle(r0)
	if handle == syscall.InvalidHandle {
		if e1 != syscall.Errno(0) {
			return syscall.InvalidHandle, e1
		}
		return syscall.InvalidHandle, syscall.EINVAL
	}
	return handle, nil
}

func connectRawNamedPipe(handle syscall.Handle) error {
	r1, _, e1 := rawConnectNamedPipe.Call(uintptr(handle), 0)
	if r1 != 0 {
		return nil
	}
	if e1 != syscall.Errno(0) {
		return e1
	}
	return syscall.EINVAL
}

func (s *winHelloAckServer) wait(t *testing.T) {
	t.Helper()

	if err := <-s.doneCh; err != nil {
		t.Fatalf("raw windows hello_ack server failed: %v", err)
	}
	if got := s.accepted.Load(); got != 1 {
		t.Fatalf("expected exactly one raw handshake accept, got %d", got)
	}
}

func startRawWinSessionServer(t *testing.T, service string, cfg windows.ServerConfig,
	handler func(*windows.Session, protocol.Header, []byte) error) *winRawSessionServer {
	return startRawWinSessionServerN(t, service, cfg, 1, handler)
}

func startRawWinSessionServerN(t *testing.T, service string, cfg windows.ServerConfig, accepts int,
	handler func(*windows.Session, protocol.Header, []byte) error) *winRawSessionServer {
	t.Helper()

	listener, err := windows.Listen(winTestRunDir, service, cfg)
	if err != nil {
		t.Fatalf("windows.Listen failed: %v", err)
	}

	srv := &winRawSessionServer{
		listener: listener,
		doneCh:   make(chan error, 1),
	}

	go func() {
		defer close(srv.doneCh)
		defer listener.Close()

		for i := 0; i < accepts; i++ {
			session, err := listener.Accept()
			if err != nil {
				srv.doneCh <- err
				return
			}

			recvBuf := make([]byte, protocol.HeaderSize+int(cfg.MaxRequestPayloadBytes))
			hdr, payload, err := session.Receive(recvBuf)
			if err != nil {
				session.Close()
				srv.doneCh <- err
				return
			}

			if err := handler(session, hdr, payload); err != nil {
				session.Close()
				srv.doneCh <- err
				return
			}

			session.Close()
		}

		srv.doneCh <- nil
	}()

	time.Sleep(200 * time.Millisecond)
	return srv
}

func (s *winRawSessionServer) wait(t *testing.T) {
	t.Helper()
	if err := <-s.doneCh; err != nil {
		t.Fatalf("raw windows session server failed: %v", err)
	}
}

func winTestCgroupsHandler(request *protocol.CgroupsRequest, builder *protocol.CgroupsBuilder) bool {
	if request.LayoutVersion != 1 || request.Flags != 0 {
		return false
	}

	builder.SetHeader(1, 42)

	items := []struct {
		hash, options, enabled uint32
		name, path             []byte
	}{
		{1001, 0, 1, []byte("docker-abc123"), []byte("/sys/fs/cgroup/docker/abc123")},
		{2002, 0, 1, []byte("k8s-pod-xyz"), []byte("/sys/fs/cgroup/kubepods/xyz")},
		{3003, 0, 0, []byte("systemd-user"), []byte("/sys/fs/cgroup/user.slice/user-1000")},
	}

	for _, item := range items {
		if err := builder.Add(item.hash, item.options, item.enabled, item.name, item.path); err != nil {
			return false
		}
	}

	return true
}

func winFailingCgroupsHandler(*protocol.CgroupsRequest, *protocol.CgroupsBuilder) bool {
	return false
}

func winIncrementDispatchHandler() DispatchHandler {
	return IncrementDispatch(func(v uint64) (uint64, bool) {
		return v + 1, true
	})
}

func winStringReverseDispatchHandler() DispatchHandler {
	return StringReverseDispatch(func(s string) (string, bool) {
		return winReverseString(s), true
	})
}

func winSnapshotDispatchHandler() DispatchHandler {
	return SnapshotDispatch(winTestCgroupsHandler, 3)
}

func winFailingSnapshotDispatchHandler() DispatchHandler {
	return SnapshotDispatch(winFailingCgroupsHandler, 3)
}
