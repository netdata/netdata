//go:build unix

package raw

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

func pingPongIncrementDispatch() DispatchHandler {
	return IncrementDispatch(func(v uint64) (uint64, bool) {
		return v + 1, true
	})
}

func pingPongStringReverseDispatch() DispatchHandler {
	return StringReverseDispatch(func(s string) (string, bool) {
		return reverseString(s), true
	})
}

// reverseString reverses a string byte-by-byte.
func reverseString(s string) string {
	b := []byte(s)
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return string(b)
}

func TestIncrementPingPong(t *testing.T) {
	svc := "go_pp_incr"
	ensureRunDir()
	cleanupAll(svc)

	ts := startTestServerUnixWithConfig(
		svc,
		testServerConfig(),
		protocol.MethodIncrement,
		pingPongIncrementDispatch(),
	)
	defer ts.stop()

	client := NewIncrementClient(testRunDir, svc, testClientConfig())
	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}

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
	cleanupAll(svc)
}

func TestStringReversePingPong(t *testing.T) {
	svc := "go_pp_strrev"
	ensureRunDir()
	cleanupAll(svc)

	ts := startTestServerUnixWithConfig(
		svc,
		testServerConfig(),
		protocol.MethodStringReverse,
		pingPongStringReverseDispatch(),
	)
	defer ts.stop()

	client := NewStringReverseClient(testRunDir, svc, testClientConfig())
	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}

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

		// verify response is the character-by-character reverse of the sent string
		expectedReversed := reverseString(current)
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
	cleanupAll(svc)
}

func TestIncrementBatch(t *testing.T) {
	svc := "go_pp_batch"
	ensureRunDir()
	cleanupAll(svc)

	// Server config with batch support
	sCfg := testServerConfig()
	sCfg.MaxRequestBatchItems = 16
	sCfg.MaxResponseBatchItems = 16
	sCfg.MaxRequestPayloadBytes = 65536

	s := NewServer(
		testRunDir,
		svc,
		sCfg,
		protocol.MethodIncrement,
		pingPongIncrementDispatch(),
	)
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		s.Run()
	}()
	defer func() {
		s.Stop()
		<-doneCh
	}()

	// Wait for server
	time.Sleep(100 * time.Millisecond)

	// Client config with batch support
	cCfg := testClientConfig()
	cCfg.MaxRequestBatchItems = 16
	cCfg.MaxResponseBatchItems = 16
	cCfg.MaxRequestPayloadBytes = 65536

	client := NewIncrementClient(testRunDir, svc, cCfg)
	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}

	input := []uint64{10, 20, 30, 40, 50}

	results, err := client.CallIncrementBatch(input)
	if err != nil {
		t.Fatalf("CallIncrementBatch failed: %v", err)
	}

	if len(results) != len(input) {
		t.Fatalf("expected %d results, got %d", len(input), len(results))
	}

	for i, v := range input {
		expected := v + 1
		if results[i] != expected {
			t.Fatalf("item %d: expected %d, got %d", i, expected, results[i])
		}
	}

	status := client.Status()
	if status.CallCount != 1 {
		t.Fatalf("expected call_count=1, got %d", status.CallCount)
	}
	if status.ErrorCount != 0 {
		t.Fatalf("expected error_count=0, got %d", status.ErrorCount)
	}

	client.Close()
	cleanupAll(svc)
}
