//go:build unix

package posix

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

// Pipelining beyond the socket buffers. Small buffers make the deadlock
// reachable on every platform (macOS and FreeBSD 14 default to 8 KiB, FreeBSD
// 15 to 64 KiB, Linux to about 208 KiB). The client's send timeout only bounds a failing run: a
// passing run never waits on it, and the server has none, so a slow server
// cannot fail a passing run.
const (
	pipeBufCount = 16
	pipeBufSize  = 16000
	pipeBufPkt   = 1024
	// Flood mode: small responses keep the client's queue bound small.
	pipeBufFloodResp = 4096
)

type smallBufMode int

const (
	smallBufEcho          smallBufMode = iota + 1 // receive one request, send its response
	smallBufCloseAfterOne                         // receive one request, then close
	smallBufFlood                                 // receive one request, then send responses until the send fails
)

func shrinkSocketBuffers(t *testing.T, fd int, boundSends bool) {
	t.Helper()
	for _, opt := range []int{syscall.SO_SNDBUF, syscall.SO_RCVBUF} {
		if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, opt, 4096); err != nil {
			t.Fatalf("setsockopt %d: %v", opt, err)
		}
	}
	if !boundSends {
		return
	}
	tv := syscall.Timeval{Sec: 10}
	if err := syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_SNDTIMEO, &tv); err != nil {
		t.Fatalf("setsockopt SO_SNDTIMEO: %v", err)
	}
}

// smallBufferStart starts the server in mode and connects a client; both
// ends get small socket buffers. The returned wait group finishes with the
// server goroutine.
func smallBufferStart(t *testing.T, mode smallBufMode, maxResponse uint32) (*Session, *sync.WaitGroup) {
	t.Helper()
	runDir := testRunDir(t)
	service := uniqueService(t)
	t.Cleanup(func() { _ = os.Remove(filepath.Join(runDir, service+".sock")) })

	sCfg := defaultServerConfig()
	sCfg.PacketSize = pipeBufPkt
	sCfg.MaxRequestPayloadBytes = 65536
	sCfg.MaxResponsePayloadBytes = maxResponse
	listener := startListener(t, runDir, service, sCfg)
	t.Cleanup(listener.Close)
	acceptCh := acceptAsync(listener)

	cCfg := defaultClientConfig()
	cCfg.PacketSize = pipeBufPkt
	cCfg.MaxRequestPayloadBytes = 65536
	cCfg.MaxResponsePayloadBytes = maxResponse
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
	shrinkSocketBuffers(t, server.Fd(), false)
	shrinkSocketBuffers(t, client.Fd(), true)

	var wg sync.WaitGroup
	wg.Go(func() {
		defer server.Close()
		count := 1
		if mode == smallBufEcho {
			count = pipeBufCount
		}
		buf := make([]byte, pipeBufPkt)
		for range count {
			hdr, payload, err := server.Receive(buf)
			if err != nil {
				return
			}
			if mode == smallBufEcho {
				resp := hdr
				resp.Kind = protocol.KindResponse
				if server.Send(&resp, payload) != nil {
					return
				}
			}
		}
		if mode == smallBufFlood {
			payload := make([]byte, pipeBufFloodResp)
			for {
				resp := protocol.Header{
					Kind:      protocol.KindResponse,
					Code:      protocol.MethodIncrement,
					ItemCount: 1,
					MessageID: 1,
				}
				if server.Send(&resp, payload) != nil {
					return
				}
			}
		}
	})
	return client, &wg
}

func pipelinedPayload(i int) []byte {
	p := make([]byte, pipeBufSize)
	for j := range p {
		p[j] = byte((i*7 + j) & 0xFF)
	}
	return p
}

func sendPipelinedRequest(s *Session, i int) error {
	hdr := protocol.Header{
		Kind:      protocol.KindRequest,
		Code:      protocol.MethodIncrement,
		ItemCount: 1,
		MessageID: uint64(i + 1),
	}
	return s.Send(&hdr, pipelinedPayload(i))
}

func sendAllPipelined(s *Session) error {
	for i := range pipeBufCount {
		if err := sendPipelinedRequest(s, i); err != nil {
			return err
		}
	}
	return nil
}

// A client that sends every request before reading any response must not
// deadlock with the server when the bytes in flight exceed the buffers.
func TestPipelineExceedsSocketBuffers(t *testing.T) {
	client, wg := smallBufferStart(t, smallBufEcho, 65536)

	if client.HasPending() {
		t.Fatal("pending data before sending")
	}
	if err := sendAllPipelined(client); err != nil {
		t.Fatalf("pipelined send: %v", err)
	}
	// The buffers hold a fraction of the responses, so finishing the sends
	// required reading responses into the queue.
	if !client.HasPending() {
		t.Fatal("no responses queued during the sends")
	}
	buf := make([]byte, pipeBufPkt)
	for i := range pipeBufCount {
		hdr, payload, err := client.Receive(buf)
		if err != nil {
			t.Fatalf("Receive[%d]: %v", i, err)
		}
		if hdr.MessageID != uint64(i+1) || !bytes.Equal(payload, pipelinedPayload(i)) {
			t.Fatalf("response %d: id %d, %d bytes", i, hdr.MessageID, len(payload))
		}
	}
	if client.HasPending() {
		t.Fatal("pending queue not drained")
	}
	// Closing (not only the shutdown a failed session does) wakes a server
	// blocked sending to it on macOS.
	client.Close()
	wg.Wait()
}

// The server closes while the pipelining client waits for buffer space: the
// send fails instead of waiting forever.
func TestPipelinePeerCloseDuringSend(t *testing.T) {
	client, wg := smallBufferStart(t, smallBufCloseAfterOne, 65536)

	if err := sendAllPipelined(client); !errors.Is(err, ErrSend) {
		t.Fatalf("expected ErrSend, got %v", err)
	}
	if len(client.inflightIDs) != 0 {
		t.Fatalf("in-flight set not cleared: %d", len(client.inflightIDs))
	}
	// Closing (not only the shutdown a failed session does) wakes a server
	// blocked sending to it on macOS.
	client.Close()
	wg.Wait()
}

// A server that sends more than the in-flight responses can occupy fails the
// client session instead of growing its queue without limit.
func TestPipelinePendingBound(t *testing.T) {
	client, wg := smallBufferStart(t, smallBufFlood, pipeBufFloodResp)

	if err := sendAllPipelined(client); !errors.Is(err, ErrProtocol) {
		t.Fatalf("expected ErrProtocol, got %v", err)
	}
	if len(client.inflightIDs) != 0 {
		t.Fatalf("in-flight set not cleared: %d", len(client.inflightIDs))
	}
	// Closing (not only the shutdown a failed session does) wakes a server
	// blocked sending to it on macOS.
	client.Close()
	wg.Wait()
}
