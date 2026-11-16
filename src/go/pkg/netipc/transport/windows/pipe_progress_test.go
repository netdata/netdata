//go:build windows

package windows

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

const (
	progressPayload  = 16 * 1024
	progressRequests = 32 // 512 KiB each way, 8x the 64 KiB pipe buffers.
)

func progressPayloadFor(id uint64) []byte {
	p := make([]byte, progressPayload)
	for i := range p {
		p[i] = byte(id) + byte(i)
	}
	return p
}

// connectProgressPair connects a client to a server that answers every
// request with its payload, one at a time, with blocking writes: the
// server of the deadlock this guards against.
func connectProgressPair(t *testing.T) (*Session, <-chan error) {
	t.Helper()
	service := uniquePipeService(t)
	scfg := defaultServerConfig()
	scfg.MaxRequestPayloadBytes = progressPayload
	scfg.MaxResponsePayloadBytes = progressPayload
	listener := startListener(t, testPipeRunDir, service, scfg)
	t.Cleanup(listener.Close)
	acceptCh := acceptAsync(listener)

	ccfg := defaultClientConfig()
	ccfg.MaxRequestPayloadBytes = progressPayload
	ccfg.MaxResponsePayloadBytes = progressPayload
	client, err := Connect(testPipeRunDir, service, &ccfg)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	t.Cleanup(client.Close)
	sr := <-acceptCh
	if sr.err != nil {
		t.Fatalf("Accept failed: %v", sr.err)
	}
	server := sr.session

	served := make(chan error, 1)
	go func() {
		defer server.Close()
		buf := make([]byte, 64*1024)
		for i := 0; i < progressRequests; i++ {
			hdr, payload, err := server.Receive(buf)
			if err != nil {
				served <- err
				return
			}
			resp := protocol.Header{Kind: protocol.KindResponse, Code: hdr.Code, ItemCount: 1, MessageID: hdr.MessageID}
			if err := server.Send(&resp, payload); err != nil {
				served <- err
				return
			}
		}
		served <- nil
	}()
	return client, served
}

// A client pipelining more than the pipe buffers before reading must not
// deadlock with a server blocked writing responses to it.
func TestPipePipelineBeyondPipeBuffers(t *testing.T) {
	client, served := connectProgressPair(t)

	done := make(chan error, 1)
	go func() {
		for id := uint64(1); id <= progressRequests; id++ {
			hdr := protocol.Header{Kind: protocol.KindRequest, Code: protocol.MethodIncrement, ItemCount: 1, MessageID: id}
			if err := client.Send(&hdr, progressPayloadFor(id)); err != nil {
				done <- err
				return
			}
		}
		// The server could read every request only because the sends
		// drained responses it had written.
		if !client.HasPending() {
			done <- errors.New("no response was queued while sending")
			return
		}
		if ready, err := client.WaitReadable(0); !ready || err != nil {
			done <- errors.New("WaitReadable does not report queued responses")
			return
		}
		buf := make([]byte, 64*1024)
		for id := uint64(1); id <= progressRequests; id++ {
			hdr, payload, err := client.Receive(buf)
			if err != nil {
				done <- err
				return
			}
			if hdr.MessageID != id || !bytes.Equal(payload, progressPayloadFor(id)) {
				done <- errors.New("response out of order or corrupted")
				return
			}
		}
		if client.HasPending() {
			done <- errors.New("responses left queued after receiving all")
			return
		}
		done <- nil
	}()

	// Watchdog only: a deadlock never completes.
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("pipelining beyond the pipe buffers deadlocked")
	}
	if err := <-served; err != nil {
		t.Fatalf("server: %v", err)
	}
}

// A queued message larger than the receive buffer fails like a pipe read
// with ERROR_MORE_DATA, and the queue moves past it.
func TestPendingQueuePopTooSmall(t *testing.T) {
	var q pendingQueue
	for _, msg := range [][]byte{[]byte("0123456789"), []byte("ab")} {
		rec := make([]byte, pendingRecordLen)
		binary.NativeEndian.PutUint32(rec, uint32(len(msg)))
		q.buf = append(q.buf, rec...)
		q.buf = append(q.buf, msg...)
	}
	if _, err := q.pop(make([]byte, 4)); !errors.Is(err, ErrRecv) {
		t.Fatalf("pop into a short buffer = %v, want ErrRecv", err)
	}
	dst := make([]byte, 4)
	n, err := q.pop(dst)
	if err != nil || string(dst[:n]) != "ab" {
		t.Fatalf("second pop = %q, %v", dst[:n], err)
	}
	if q.queued() {
		t.Fatal("queue not empty")
	}
}

// A message larger than the receive buffer fails on the overlapped client
// handle too, instead of returning its first part as a message.
func TestPipeClientReceiveMoreDataFails(t *testing.T) {
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

	if err := rawWrite(sr.session.handle, make([]byte, 96)); err != nil {
		t.Fatalf("rawWrite: %v", err)
	}
	if _, err := client.readPacket(make([]byte, 16)); !errors.Is(err, ErrRecv) {
		t.Fatalf("readPacket into a short buffer = %v, want ErrRecv", err)
	}
}
