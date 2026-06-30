//go:build unix

package posix

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

// ---------------------------------------------------------------------------
//  Receive: payload_len exceeds negotiated limit
// ---------------------------------------------------------------------------

func TestReceivePayloadExceedsLimit(t *testing.T) {
	runDir := testRunDir(t)
	service := uniqueService(t)
	defer os.Remove(filepath.Join(runDir, service+".sock"))

	// Server allows up to 128 bytes request payload
	sCfg := defaultServerConfig()
	sCfg.MaxRequestPayloadBytes = 128
	sCfg.MaxResponsePayloadBytes = 128
	listener := startListener(t, runDir, service, sCfg)
	defer listener.Close()

	acceptCh := acceptAsync(listener)

	// Client also advertises 128 so negotiation settles on 128
	cCfg := defaultClientConfig()
	cCfg.MaxRequestPayloadBytes = 128
	cCfg.MaxResponsePayloadBytes = 128
	client, err := Connect(runDir, service, &cCfg)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	sr := <-acceptCh
	if sr.err != nil {
		t.Fatalf("Accept: %v", sr.err)
	}
	server := sr.session
	defer server.Close()

	// Client sends request with message_id=1, server receives OK
	hdr := protocol.Header{
		Kind:      protocol.KindRequest,
		Code:      protocol.MethodIncrement,
		ItemCount: 1,
		MessageID: 1,
	}
	if err := client.Send(&hdr, []byte("ok")); err != nil {
		t.Fatalf("Send: %v", err)
	}

	buf := make([]byte, 4096)
	_, _, err = server.Receive(buf)
	if err != nil {
		t.Fatalf("server Receive: %v", err)
	}

	// Server sends response with artificially large payload that exceeds
	// the client's negotiated limit. We need to forge this via raw send
	// since session.Send doesn't validate outgoing size.
	resp := protocol.Header{
		Kind:      protocol.KindResponse,
		Code:      protocol.MethodIncrement,
		ItemCount: 1,
		MessageID: 1,
	}
	// Forge a payload > 128 bytes
	bigPayload := make([]byte, 200)
	if err := server.Send(&resp, bigPayload); err != nil {
		t.Fatalf("server Send: %v", err)
	}

	// Client receive should fail with limit exceeded
	_, _, err = client.Receive(buf)
	if err == nil {
		t.Fatal("expected limit exceeded error")
	}
	if !errors.Is(err, ErrLimitExceeded) {
		t.Errorf("error = %v, want ErrLimitExceeded", err)
	}
}

// ---------------------------------------------------------------------------
//  Receive: item_count exceeds negotiated batch limit
// ---------------------------------------------------------------------------

func TestReceiveBatchExceedsLimit(t *testing.T) {
	runDir := testRunDir(t)
	service := uniqueService(t)
	defer os.Remove(filepath.Join(runDir, service+".sock"))

	// Server allows batch items = 2
	sCfg := defaultServerConfig()
	sCfg.MaxRequestBatchItems = 2
	sCfg.MaxResponseBatchItems = 2
	listener := startListener(t, runDir, service, sCfg)
	defer listener.Close()

	acceptCh := acceptAsync(listener)

	cCfg := defaultClientConfig()
	cCfg.MaxRequestBatchItems = 2
	cCfg.MaxResponseBatchItems = 2
	client, err := Connect(runDir, service, &cCfg)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	sr := <-acceptCh
	if sr.err != nil {
		t.Fatalf("Accept: %v", sr.err)
	}
	server := sr.session
	defer server.Close()

	// Client sends a valid request
	hdr := protocol.Header{
		Kind:      protocol.KindRequest,
		Code:      protocol.MethodIncrement,
		ItemCount: 1,
		MessageID: 1,
	}
	if err := client.Send(&hdr, []byte("x")); err != nil {
		t.Fatalf("Send: %v", err)
	}

	buf := make([]byte, 4096)
	_, _, err = server.Receive(buf)
	if err != nil {
		t.Fatalf("server Receive: %v", err)
	}

	// Server responds with item_count=10 (exceeds negotiated 2)
	resp := protocol.Header{
		Kind:      protocol.KindResponse,
		Code:      protocol.MethodIncrement,
		ItemCount: 10,
		MessageID: 1,
	}
	if err := server.Send(&resp, []byte("y")); err != nil {
		t.Fatalf("server Send: %v", err)
	}

	// Client receive should fail
	_, _, err = client.Receive(buf)
	if err == nil {
		t.Fatal("expected limit exceeded error for batch items")
	}
	if !errors.Is(err, ErrLimitExceeded) {
		t.Errorf("error = %v, want ErrLimitExceeded", err)
	}
}

// ---------------------------------------------------------------------------
//  Handshake: client gets non-OK transport_status (generic)
// ---------------------------------------------------------------------------

func TestWrapErr(t *testing.T) {
	// Verify wrapErr produces a properly wrapped error
	err := wrapErr(ErrConnect, "some detail")
	if !errors.Is(err, ErrConnect) {
		t.Error("wrapErr result should wrap ErrConnect")
	}
	if err.Error() != "connect failed: some detail" {
		t.Errorf("unexpected error string: %q", err.Error())
	}

	err2 := wrapErr(ErrAuthFailed, "bad token")
	if !errors.Is(err2, ErrAuthFailed) {
		t.Error("wrapErr result should wrap ErrAuthFailed")
	}
}

// ---------------------------------------------------------------------------
//  validateServiceName edge cases
// ---------------------------------------------------------------------------

func TestValidateServiceNameEdgeCases(t *testing.T) {
	// Valid names
	valid := []string{"a", "A", "0", "test-service", "test.service", "test_service", "Test123"}
	for _, name := range valid {
		if err := validateServiceName(name); err != nil {
			t.Errorf("validateServiceName(%q) = %v, want nil", name, err)
		}
	}

	// Invalid names
	invalid := []string{"", ".", "..", "a/b", "a b", "a\tb", "a@b", "a#b", "a$b", "a!b"}
	for _, name := range invalid {
		if err := validateServiceName(name); err == nil {
			t.Errorf("validateServiceName(%q) = nil, want error", name)
		}
	}
}

// ---------------------------------------------------------------------------
//  buildSocketPath edge cases
// ---------------------------------------------------------------------------

func TestBuildSocketPathEdgeCases(t *testing.T) {
	// Valid case
	path, err := buildSocketPath("/tmp", "test")
	if err != nil {
		t.Fatalf("buildSocketPath: %v", err)
	}
	expected := filepath.Join("/tmp", "test.sock")
	if path != expected {
		t.Fatalf("path = %q, want %q", path, expected)
	}

	// Invalid service name
	_, err = buildSocketPath("/tmp", "")
	if err == nil {
		t.Fatal("expected error for empty service name")
	}

	// Path too long
	longDir := "/tmp/" + string(make([]byte, 200))
	_, err = buildSocketPath(longDir, "test")
	if err != ErrPathTooLong {
		t.Fatalf("expected ErrPathTooLong, got %v", err)
	}
}

// ---------------------------------------------------------------------------
//  applyDefault and minU32 helpers
// ---------------------------------------------------------------------------

func TestApplyDefault(t *testing.T) {
	if got := applyDefault(0, 42); got != 42 {
		t.Errorf("applyDefault(0, 42) = %d, want 42", got)
	}
	if got := applyDefault(10, 42); got != 10 {
		t.Errorf("applyDefault(10, 42) = %d, want 10", got)
	}
}

func TestMinU32(t *testing.T) {
	if got := minU32(5, 10); got != 5 {
		t.Errorf("minU32(5, 10) = %d, want 5", got)
	}
	if got := minU32(10, 5); got != 5 {
		t.Errorf("minU32(10, 5) = %d, want 5", got)
	}
	if got := minU32(7, 7); got != 7 {
		t.Errorf("minU32(7, 7) = %d, want 7", got)
	}
}

// ---------------------------------------------------------------------------
//  Session.Close double-close safety
// ---------------------------------------------------------------------------

func TestSessionDoubleClose(t *testing.T) {
	runDir := testRunDir(t)
	service := uniqueService(t)
	defer os.Remove(filepath.Join(runDir, service+".sock"))

	sCfg := defaultServerConfig()
	listener := startListener(t, runDir, service, sCfg)
	defer listener.Close()

	acceptCh := acceptAsync(listener)

	cCfg := defaultClientConfig()
	client, err := Connect(runDir, service, &cCfg)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	sr := <-acceptCh
	if sr.err != nil {
		t.Fatalf("Accept: %v", sr.err)
	}
	sr.session.Close()

	// Double close should not panic
	client.Close()
	client.Close()
}

// ---------------------------------------------------------------------------
//  Listener.Close double-close safety
// ---------------------------------------------------------------------------

func TestListenerDoubleClose(t *testing.T) {
	runDir := testRunDir(t)
	service := uniqueService(t)

	sCfg := defaultServerConfig()
	listener := startListener(t, runDir, service, sCfg)

	// Double close should not panic
	listener.Close()
	listener.Close()
}
