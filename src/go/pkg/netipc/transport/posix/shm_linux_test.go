//go:build linux

package posix

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

const testShmRunDir = "/tmp/nipc_shm_go_test"

func ensureShmRunDir(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(testShmRunDir, 0700); err != nil {
		t.Fatalf("cannot create SHM run dir: %v", err)
	}
}

func cleanupShmFiles(t *testing.T, service string) {
	t.Helper()
	// Clean up SHM files with session ID pattern
	entries, _ := os.ReadDir(testShmRunDir)
	for _, e := range entries {
		name := e.Name()
		if len(name) > len(service)+1 && name[:len(service)+1] == service+"-" {
			os.Remove(fmt.Sprintf("%s/%s", testShmRunDir, name))
		}
	}
}

func uniqueShmService(t *testing.T, prefix string) string {
	t.Helper()
	return fmt.Sprintf("%s_%s", prefix, uniqueService(t))
}

func waitShmClientAttach(t *testing.T, runDir, service string, sessionID uint64) *ShmContext {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		ctx, err := ShmClientAttach(runDir, service, sessionID)
		if err == nil {
			return ctx
		}
		lastErr = err

		if !errors.Is(err, ErrShmOpen) &&
			!errors.Is(err, ErrShmNotReady) &&
			!errors.Is(err, ErrShmBadMagic) &&
			!errors.Is(err, ErrShmBadVersion) &&
			!errors.Is(err, ErrShmBadHeader) &&
			!errors.Is(err, ErrShmBadSize) {
			t.Fatalf("attach should not fail with non-transient error while waiting for readiness: %v", err)
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for SHM region readiness: %v", lastErr)
	return nil
}

// buildShmMessage creates a complete wire message (32-byte header + payload).
func buildShmMessage(kind, code uint16, messageID uint64, payload []byte) []byte {
	hdr := protocol.Header{
		Magic:      protocol.MagicMsg,
		Version:    protocol.Version,
		HeaderLen:  protocol.HeaderLen,
		Kind:       kind,
		Code:       code,
		ItemCount:  1,
		MessageID:  messageID,
		PayloadLen: uint32(len(payload)),
	}
	buf := make([]byte, protocol.HeaderSize+len(payload))
	hdr.Encode(buf[:protocol.HeaderSize])
	copy(buf[protocol.HeaderSize:], payload)
	return buf
}

func TestShmDirectRoundtrip(t *testing.T) {
	ensureShmRunDir(t)
	svc := uniqueShmService(t, "go_shm_rt")
	cleanupShmFiles(t, svc)
	defer cleanupShmFiles(t, svc)

	var wg sync.WaitGroup
	var serverErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx, err := ShmServerCreate(testShmRunDir, svc, 1, 4096, 4096)
		if err != nil {
			serverErr = fmt.Errorf("server create: %w", err)
			return
		}
		defer ctx.ShmDestroy()

		buf := make([]byte, 65536)
		mlen, err := ctx.ShmReceive(buf, 5000)
		if err != nil {
			serverErr = fmt.Errorf("server receive: %w", err)
			return
		}

		if mlen < protocol.HeaderSize {
			serverErr = fmt.Errorf("message too short: %d", mlen)
			return
		}

		// Parse header, echo as response
		hdr, err := protocol.DecodeHeader(buf[:mlen])
		if err != nil {
			serverErr = fmt.Errorf("decode header: %w", err)
			return
		}
		payload := make([]byte, mlen-protocol.HeaderSize)
		copy(payload, buf[protocol.HeaderSize:mlen])
		resp := buildShmMessage(protocol.KindResponse, hdr.Code, hdr.MessageID, payload)
		if err := ctx.ShmSend(resp); err != nil {
			serverErr = fmt.Errorf("server send: %w", err)
		}
	}()

	client := waitShmClientAttach(t, testShmRunDir, svc, 1)
	defer client.ShmClose()

	payload := []byte{0xCA, 0xFE, 0xBA, 0xBE}
	msg := buildShmMessage(protocol.KindRequest, protocol.MethodIncrement, 42, payload)
	if err := client.ShmSend(msg); err != nil {
		t.Fatalf("client send: %v", err)
	}

	respBuf := make([]byte, 65536)
	rlen, err := client.ShmReceive(respBuf, 5000)
	if err != nil {
		t.Fatalf("client receive: %v", err)
	}

	if rlen != protocol.HeaderSize+len(payload) {
		t.Fatalf("response length: got %d, want %d", rlen, protocol.HeaderSize+len(payload))
	}

	rhdr, err := protocol.DecodeHeader(respBuf[:rlen])
	if err != nil {
		t.Fatalf("decode response header: %v", err)
	}
	if rhdr.Kind != protocol.KindResponse {
		t.Errorf("response kind: got %d, want %d", rhdr.Kind, protocol.KindResponse)
	}
	if rhdr.MessageID != 42 {
		t.Errorf("response message_id: got %d, want 42", rhdr.MessageID)
	}
	respPayload := respBuf[protocol.HeaderSize:rlen]
	if !bytes.Equal(respPayload, payload) {
		t.Errorf("response payload mismatch")
	}

	wg.Wait()
	if serverErr != nil {
		t.Fatalf("server error: %v", serverErr)
	}
}

func TestShmMultipleRoundtrips(t *testing.T) {
	ensureShmRunDir(t)
	svc := uniqueShmService(t, "go_shm_multi")
	cleanupShmFiles(t, svc)
	defer cleanupShmFiles(t, svc)

	var wg sync.WaitGroup
	var serverErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx, err := ShmServerCreate(testShmRunDir, svc, 2, 4096, 4096)
		if err != nil {
			serverErr = fmt.Errorf("server create: %w", err)
			return
		}
		defer ctx.ShmDestroy()

		buf := make([]byte, 65536)
		for i := 0; i < 10; i++ {
			mlen, err := ctx.ShmReceive(buf, 5000)
			if err != nil {
				serverErr = fmt.Errorf("server receive %d: %w", i, err)
				return
			}
			hdr, err := protocol.DecodeHeader(buf[:mlen])
			if err != nil {
				serverErr = fmt.Errorf("decode header %d: %w", i, err)
				return
			}
			payload := make([]byte, mlen-protocol.HeaderSize)
			copy(payload, buf[protocol.HeaderSize:mlen])
			resp := buildShmMessage(protocol.KindResponse, hdr.Code, hdr.MessageID, payload)
			if err := ctx.ShmSend(resp); err != nil {
				serverErr = fmt.Errorf("server send %d: %w", i, err)
				return
			}
		}
	}()

	client := waitShmClientAttach(t, testShmRunDir, svc, 2)
	defer client.ShmClose()

	respBuf := make([]byte, 65536)
	for i := uint64(0); i < 10; i++ {
		payload := []byte{byte(i)}
		msg := buildShmMessage(protocol.KindRequest, 1, i+1, payload)
		if err := client.ShmSend(msg); err != nil {
			t.Fatalf("client send %d: %v", i, err)
		}

		rlen, err := client.ShmReceive(respBuf, 5000)
		if err != nil {
			t.Fatalf("client receive %d: %v", i, err)
		}

		rhdr, err := protocol.DecodeHeader(respBuf[:rlen])
		if err != nil {
			t.Fatalf("decode response %d: %v", i, err)
		}
		if rhdr.Kind != protocol.KindResponse {
			t.Errorf("round %d: kind=%d, want %d", i, rhdr.Kind, protocol.KindResponse)
		}
		if rhdr.MessageID != i+1 {
			t.Errorf("round %d: message_id=%d, want %d", i, rhdr.MessageID, i+1)
		}
		if respBuf[protocol.HeaderSize] != byte(i) {
			t.Errorf("round %d: payload byte=%d, want %d", i, respBuf[protocol.HeaderSize], i)
		}
	}

	wg.Wait()
	if serverErr != nil {
		t.Fatalf("server error: %v", serverErr)
	}
}

func TestShmStaleRecovery(t *testing.T) {
	ensureShmRunDir(t)
	svc := uniqueShmService(t, "go_shm_stale")
	cleanupShmFiles(t, svc)
	defer cleanupShmFiles(t, svc)

	// Create a region, then corrupt owner_pid to simulate dead process
	first, err := ShmServerCreate(testShmRunDir, svc, 3, 1024, 1024)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	// Write a dead PID into the header
	binary.NativeEndian.PutUint32(first.data[8:12], 99999) // very unlikely alive
	first.ShmClose()                                       // close without unlink

	// Clean up stale regions (as production server would)
	ShmCleanupStale(testShmRunDir, svc)

	// Should succeed after stale recovery
	second, err := ShmServerCreate(testShmRunDir, svc, 3, 2048, 2048)
	if err != nil {
		t.Fatalf("stale recovery create: %v", err)
	}
	if second.requestCapacity < 2048 {
		t.Errorf("new region capacity: %d, want >= 2048", second.requestCapacity)
	}
	second.ShmDestroy()
}

func TestShmClientAttachPartialHeaderNotReady(t *testing.T) {
	ensureShmRunDir(t)
	svc := uniqueShmService(t, "go_shm_partial")
	cleanupShmFiles(t, svc)
	defer cleanupShmFiles(t, svc)

	server, err := ShmServerCreate(testShmRunDir, svc, 5, 1024, 1024)
	if err != nil {
		t.Fatalf("server create: %v", err)
	}
	defer server.ShmDestroy()

	reqOff := binary.NativeEndian.Uint32(server.data[shmHeaderReqOffOff : shmHeaderReqOffOff+4])
	reqCap := binary.NativeEndian.Uint32(server.data[shmHeaderReqCapOff : shmHeaderReqCapOff+4])
	respOff := binary.NativeEndian.Uint32(server.data[shmHeaderRespOffOff : shmHeaderRespOffOff+4])
	respCap := binary.NativeEndian.Uint32(server.data[shmHeaderRespCapOff : shmHeaderRespCapOff+4])

	binary.NativeEndian.PutUint32(server.data[shmHeaderReqOffOff:shmHeaderReqOffOff+4], 0)
	binary.NativeEndian.PutUint32(server.data[shmHeaderReqCapOff:shmHeaderReqCapOff+4], 0)
	binary.NativeEndian.PutUint32(server.data[shmHeaderRespOffOff:shmHeaderRespOffOff+4], 0)
	binary.NativeEndian.PutUint32(server.data[shmHeaderRespCapOff:shmHeaderRespCapOff+4], 0)

	_, err = ShmClientAttach(testShmRunDir, svc, 5)
	if !errors.Is(err, ErrShmNotReady) {
		t.Fatalf("client attach error = %v, want %v", err, ErrShmNotReady)
	}

	binary.NativeEndian.PutUint32(server.data[shmHeaderReqOffOff:shmHeaderReqOffOff+4], reqOff)
	binary.NativeEndian.PutUint32(server.data[shmHeaderReqCapOff:shmHeaderReqCapOff+4], reqCap)
	binary.NativeEndian.PutUint32(server.data[shmHeaderRespOffOff:shmHeaderRespOffOff+4], respOff)
	binary.NativeEndian.PutUint32(server.data[shmHeaderRespCapOff:shmHeaderRespCapOff+4], respCap)
}

func TestShmLargeMessage(t *testing.T) {
	ensureShmRunDir(t)
	svc := uniqueShmService(t, "go_shm_large")
	cleanupShmFiles(t, svc)
	defer cleanupShmFiles(t, svc)

	var wg sync.WaitGroup
	var serverErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx, err := ShmServerCreate(testShmRunDir, svc, 4, 65536, 65536)
		if err != nil {
			serverErr = fmt.Errorf("server create: %w", err)
			return
		}
		defer ctx.ShmDestroy()

		buf := make([]byte, 65536)
		mlen, err := ctx.ShmReceive(buf, 5000)
		if err != nil {
			serverErr = fmt.Errorf("server receive: %w", err)
			return
		}

		hdr, err := protocol.DecodeHeader(buf[:mlen])
		if err != nil {
			serverErr = fmt.Errorf("decode: %w", err)
			return
		}
		payload := make([]byte, mlen-protocol.HeaderSize)
		copy(payload, buf[protocol.HeaderSize:mlen])
		resp := buildShmMessage(protocol.KindResponse, hdr.Code, hdr.MessageID, payload)
		if err := ctx.ShmSend(resp); err != nil {
			serverErr = fmt.Errorf("server send: %w", err)
		}
	}()

	client := waitShmClientAttach(t, testShmRunDir, svc, 4)
	defer client.ShmClose()

	// 60000 bytes of payload
	payload := make([]byte, 60000)
	for i := range payload {
		payload[i] = byte(i & 0xFF)
	}
	msg := buildShmMessage(protocol.KindRequest, 1, 999, payload)
	if err := client.ShmSend(msg); err != nil {
		t.Fatalf("client send: %v", err)
	}

	respBuf := make([]byte, 65536)
	rlen, err := client.ShmReceive(respBuf, 5000)
	if err != nil {
		t.Fatalf("client receive: %v", err)
	}

	if rlen != protocol.HeaderSize+len(payload) {
		t.Fatalf("response length: got %d, want %d", rlen, protocol.HeaderSize+len(payload))
	}

	respPayload := respBuf[protocol.HeaderSize:rlen]
	if !bytes.Equal(respPayload, payload) {
		t.Errorf("response payload pattern mismatch")
	}

	wg.Wait()
	if serverErr != nil {
		t.Fatalf("server error: %v", serverErr)
	}
}

func TestShmChaosForgedLength(t *testing.T) {
	ensureShmRunDir(t)
	svc := uniqueShmService(t, "go_shm_forged")
	cleanupShmFiles(t, svc)
	defer cleanupShmFiles(t, svc)

	const reqCap uint32 = 1024
	const respCap uint32 = 1024

	// --- Server-side receive with forged req_len ---

	srv, err := ShmServerCreate(testShmRunDir, svc, 100, reqCap, respCap)
	if err != nil {
		t.Fatalf("server create: %v", err)
	}
	defer srv.ShmDestroy()

	// Attach a client so we can also test client-side forged resp_len
	client, err := ShmClientAttach(testShmRunDir, svc, 100)
	if err != nil {
		t.Fatalf("client attach: %v", err)
	}
	defer client.ShmClose()

	buf := make([]byte, 65536)

	// Test forged req_len values on the server side.
	// We directly manipulate the mapped region to simulate a malicious client.
	forgedLengths := []uint32{0, reqCap + 1, 0xFFFFFFFF}

	for _, forgedLen := range forgedLengths {
		// Write garbage into the request area
		for i := srv.requestOffset; i < srv.requestOffset+reqCap; i++ {
			srv.data[i] = 0xAA
		}

		// Store forged req_len
		if err := atomicStoreU32(srv.data, shmHeaderReqLenOff, forgedLen); err != nil {
			t.Fatalf("store forged req_len=%d: %v", forgedLen, err)
		}

		// Bump req_seq to signal a "message" arrived
		if err := atomicAddU64(srv.data, shmHeaderReqSeqOff, 1); err != nil {
			t.Fatalf("add req_seq for forged=%d: %v", forgedLen, err)
		}

		// Bump req_signal to wake futex
		if err := atomicAddU32(srv.data, shmHeaderReqSignalOff, 1); err != nil {
			t.Fatalf("add req_signal for forged=%d: %v", forgedLen, err)
		}

		mlen, recvErr := srv.ShmReceive(buf, 1000)

		if forgedLen == 0 {
			// Zero-length: no copy, no error, returns 0 bytes
			if recvErr != nil {
				t.Errorf("forged req_len=0: unexpected error: %v", recvErr)
			}
			if mlen != 0 {
				t.Errorf("forged req_len=0: got mlen=%d, want 0", mlen)
			}
		} else {
			// Oversized: must return ErrShmMsgTooLarge, no panic
			if !errors.Is(recvErr, ErrShmMsgTooLarge) {
				t.Errorf("forged req_len=%d: got err=%v, want ErrShmMsgTooLarge", forgedLen, recvErr)
			}
		}
	}

	// --- Client-side receive with forged resp_len ---

	forgedRespLengths := []uint32{0, respCap + 1, 0xFFFFFFFF}

	for _, forgedLen := range forgedRespLengths {
		// Write garbage into the response area
		for i := srv.responseOffset; i < srv.responseOffset+respCap; i++ {
			srv.data[i] = 0xBB
		}

		// Store forged resp_len (server and client share the same mapped region)
		if err := atomicStoreU32(client.data, shmHeaderRespLenOff, forgedLen); err != nil {
			t.Fatalf("store forged resp_len=%d: %v", forgedLen, err)
		}

		// Bump resp_seq
		if err := atomicAddU64(client.data, shmHeaderRespSeqOff, 1); err != nil {
			t.Fatalf("add resp_seq for forged=%d: %v", forgedLen, err)
		}

		// Bump resp_signal
		if err := atomicAddU32(client.data, shmHeaderRespSignalOff, 1); err != nil {
			t.Fatalf("add resp_signal for forged=%d: %v", forgedLen, err)
		}

		mlen, recvErr := client.ShmReceive(buf, 1000)

		if forgedLen == 0 {
			if recvErr != nil {
				t.Errorf("forged resp_len=0: unexpected error: %v", recvErr)
			}
			if mlen != 0 {
				t.Errorf("forged resp_len=0: got mlen=%d, want 0", mlen)
			}
		} else {
			if !errors.Is(recvErr, ErrShmMsgTooLarge) {
				t.Errorf("forged resp_len=%d: got err=%v, want ErrShmMsgTooLarge", forgedLen, recvErr)
			}
		}
	}
}

func TestShmMultiClient(t *testing.T) {
	ensureShmRunDir(t)
	svc := uniqueShmService(t, "go_shm_mcli")
	cleanupShmFiles(t, svc)
	defer cleanupShmFiles(t, svc)

	const numClients = 3

	type serverSlot struct {
		ctx *ShmContext
		err error
		got []byte // received payload
	}

	var wg sync.WaitGroup
	slots := make([]serverSlot, numClients)

	// Create server regions and start goroutines that receive + echo
	for i := 0; i < numClients; i++ {
		sessionID := uint64(i + 1)
		ctx, err := ShmServerCreate(testShmRunDir, svc, sessionID, 4096, 4096)
		if err != nil {
			// Clean up already-created regions
			for j := 0; j < i; j++ {
				slots[j].ctx.ShmDestroy()
			}
			t.Fatalf("server create session %d: %v", sessionID, err)
		}
		slots[i].ctx = ctx

		idx := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 65536)
			mlen, err := slots[idx].ctx.ShmReceive(buf, 5000)
			if err != nil {
				slots[idx].err = fmt.Errorf("receive: %w", err)
				return
			}
			// Save received payload for verification
			slots[idx].got = make([]byte, mlen)
			copy(slots[idx].got, buf[:mlen])

			// Parse and echo back as response
			hdr, err := protocol.DecodeHeader(buf[:mlen])
			if err != nil {
				slots[idx].err = fmt.Errorf("decode: %w", err)
				return
			}
			payload := make([]byte, mlen-protocol.HeaderSize)
			copy(payload, buf[protocol.HeaderSize:mlen])
			resp := buildShmMessage(protocol.KindResponse, hdr.Code, hdr.MessageID, payload)
			if err := slots[idx].ctx.ShmSend(resp); err != nil {
				slots[idx].err = fmt.Errorf("send: %w", err)
			}
		}()
	}

	// Attach clients and send unique messages
	clients := make([]*ShmContext, numClients)
	payloads := make([][]byte, numClients)
	for i := 0; i < numClients; i++ {
		sessionID := uint64(i + 1)
		c := waitShmClientAttach(t, testShmRunDir, svc, sessionID)
		defer c.ShmClose()
		clients[i] = c

		// Each client sends a unique payload: [0xC0+i, session_id_byte, 0xDE, 0xAD]
		payloads[i] = []byte{byte(0xC0 + i), byte(sessionID), 0xDE, 0xAD}
		msg := buildShmMessage(protocol.KindRequest, protocol.MethodIncrement, uint64(100+i), payloads[i])
		if err := c.ShmSend(msg); err != nil {
			t.Fatalf("client %d send: %v", i, err)
		}
	}

	// Each client receives its own response
	for i := 0; i < numClients; i++ {
		respBuf := make([]byte, 65536)
		rlen, err := clients[i].ShmReceive(respBuf, 5000)
		if err != nil {
			t.Fatalf("client %d receive: %v", i, err)
		}

		rhdr, err := protocol.DecodeHeader(respBuf[:rlen])
		if err != nil {
			t.Fatalf("client %d decode response: %v", i, err)
		}

		if rhdr.Kind != protocol.KindResponse {
			t.Errorf("client %d: kind=%d, want %d", i, rhdr.Kind, protocol.KindResponse)
		}
		if rhdr.MessageID != uint64(100+i) {
			t.Errorf("client %d: message_id=%d, want %d", i, rhdr.MessageID, 100+i)
		}

		respPayload := respBuf[protocol.HeaderSize:rlen]
		if !bytes.Equal(respPayload, payloads[i]) {
			t.Errorf("client %d: payload mismatch: got %x, want %x", i, respPayload, payloads[i])
		}
	}

	// Wait for server goroutines to finish
	wg.Wait()

	// Check for server errors and verify no cross-contamination
	for i := 0; i < numClients; i++ {
		if slots[i].err != nil {
			t.Errorf("server %d error: %v", i, slots[i].err)
			continue
		}
		// Verify each server got the right client's message (check payload)
		if len(slots[i].got) < protocol.HeaderSize+len(payloads[i]) {
			t.Errorf("server %d: received too few bytes: %d", i, len(slots[i].got))
			continue
		}
		srvPayload := slots[i].got[protocol.HeaderSize:]
		if !bytes.Equal(srvPayload, payloads[i]) {
			t.Errorf("server %d: cross-contamination: got %x, want %x", i, srvPayload, payloads[i])
		}
	}

	// Cleanup all server regions
	for i := 0; i < numClients; i++ {
		slots[i].ctx.ShmDestroy()
	}
}
