//go:build windows

package raw

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	windows "github.com/netdata/netdata/go/plugins/pkg/netipc/transport/windows"
)

const (
	winTestRunDir      = `C:\ProgramData\netipc_test`
	winAuthToken       = uint64(0xDEADBEEFCAFEBABE)
	winResponseBufSize = 65536
)

// testWinServerConfig returns a baseline Windows server config for tests.
func testWinServerConfig() windows.ServerConfig {
	return windows.ServerConfig{
		SupportedProfiles:       protocol.ProfileBaseline,
		MaxRequestPayloadBytes:  4096,
		MaxRequestBatchItems:    1,
		MaxResponsePayloadBytes: winResponseBufSize,
		MaxResponseBatchItems:   1,
		AuthToken:               winAuthToken,
	}
}

// testWinClientConfig returns a baseline Windows client config for tests.
func testWinClientConfig() windows.ClientConfig {
	return windows.ClientConfig{
		SupportedProfiles:       protocol.ProfileBaseline,
		MaxRequestPayloadBytes:  4096,
		MaxRequestBatchItems:    1,
		MaxResponsePayloadBytes: winResponseBufSize,
		MaxResponseBatchItems:   1,
		AuthToken:               winAuthToken,
	}
}

// winTestServer wraps a Server with a done channel for clean shutdown.
type winTestServer struct {
	server *Server
	doneCh chan struct{}
}

// startTestServerWin creates a Windows Server (Named Pipe transport)
// and starts it in a background goroutine. Returns a handle for cleanup.
func startTestServerWin(service string, expectedMethodCode uint16, handler DispatchHandler) *winTestServer {
	s := NewServer(winTestRunDir, service, testWinServerConfig(), expectedMethodCode, handler)
	doneCh := make(chan struct{})

	go func() {
		defer close(doneCh)
		s.Run()
	}()

	// Allow the Named Pipe listener to start
	time.Sleep(200 * time.Millisecond)

	return &winTestServer{server: s, doneCh: doneCh}
}

func (ts *winTestServer) stop() {
	ts.server.Stop()
	<-ts.doneCh
}

func winPingPongIncrementDispatch() DispatchHandler {
	return IncrementDispatch(func(v uint64) (uint64, bool) {
		return v + 1, true
	})
}

func winPingPongStringReverseDispatch() DispatchHandler {
	return StringReverseDispatch(func(s string) (string, bool) {
		return winReverseString(s), true
	})
}

// winReverseString reverses a string byte-by-byte.
func winReverseString(s string) string {
	b := []byte(s)
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return string(b)
}

func TestWinIncrementPingPong(t *testing.T) {
	svc := "go_win_pp_incr"

	ts := startTestServerWin(svc, protocol.MethodIncrement, winPingPongIncrementDispatch())
	defer ts.stop()

	client := NewIncrementClient(winTestRunDir, svc, testWinClientConfig())
	waitWinClientReady(t, client)

	// 10 rounds: send 0 -> get 1 -> send 1 -> get 2 -> ... -> value == 10
	var val uint64
	responsesReceived := 0
	for i := 0; i < 10; i++ {
		got, err := client.CallIncrement(val)
		if err != nil {
			t.Fatalf("round %d: CallIncrement(%d) failed: %v", i, val, err)
		}
		responsesReceived++
		expected := val + 1
		if got != expected {
			t.Fatalf("round %d: expected %d, got %d", i, expected, got)
		}
		val = got
	}

	if responsesReceived != 10 {
		t.Fatalf("expected 10 responses received, got %d", responsesReceived)
	}
	if val != 10 {
		t.Fatalf("expected final value 10, got %d", val)
	}

	status := client.Status()
	if status.CallCount != 10 {
		t.Fatalf("expected call_count=10, got %d", status.CallCount)
	}
	if status.ErrorCount != 0 {
		t.Fatalf("expected error_count=0, got %d", status.ErrorCount)
	}

	client.Close()
}

func TestWinStringReversePingPong(t *testing.T) {
	svc := "go_win_pp_strrev"

	ts := startTestServerWin(svc, protocol.MethodStringReverse, winPingPongStringReverseDispatch())
	defer ts.stop()

	client := NewStringReverseClient(winTestRunDir, svc, testWinClientConfig())
	waitWinClientReady(t, client)

	original := "abcdefghijklmnopqrstuvwxyz"

	// 6 rounds: feed each response back as the next request
	responsesReceived := 0
	current := original
	for i := 0; i < 6; i++ {
		view, err := client.CallStringReverse(current)
		if err != nil {
			t.Fatalf("round %d: CallStringReverse(%q) failed: %v", i+1, current, err)
		}
		responsesReceived++

		expectedReversed := winReverseString(current)
		if view.Str != expectedReversed {
			t.Fatalf("round %d: sent %q, expected reverse %q, got %q", i+1, current, expectedReversed, view.Str)
		}

		current = view.Str
	}

	if responsesReceived != 6 {
		t.Fatalf("expected 6 responses received, got %d", responsesReceived)
	}

	// even number of reversals = identity
	if current != original {
		t.Fatalf("after 6 reversals expected original %q, got %q", original, current)
	}

	status := client.Status()
	if status.CallCount != 6 {
		t.Fatalf("expected call_count=6, got %d", status.CallCount)
	}
	if status.ErrorCount != 0 {
		t.Fatalf("expected error_count=0, got %d", status.ErrorCount)
	}

	client.Close()
}
