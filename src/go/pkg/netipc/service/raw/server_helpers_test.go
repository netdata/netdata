package raw

import (
	"fmt"
	"testing"
	"time"
)

// testServerTimings keep timeout tests in milliseconds while staying well
// above scheduling noise.
var testServerTimings = serverTimings{
	pollMs:      10,
	stopGrace:   200 * time.Millisecond,
	handshakeMs: 500,
}

// testShmAttachTimeout replaces the 5 s production SHM attach window.
const testShmAttachTimeout = 300 * time.Millisecond

// testWaitLimit bounds waits that only expire when the code under test is
// broken; reaching it fails the test.
const testWaitLimit = 10 * time.Second

// runServerForTest runs s with test timings and returns once it accepts
// connections. It panics, failing the test binary loudly, if Run exits or
// never binds.
func runServerForTest(s *Server) chan struct{} {
	s.timings = testServerTimings
	s.sessionExited = make(chan struct{}, 64)
	s.handshakeStarted = make(chan struct{}, 64)
	s.shmCreated = make(chan struct{}, 64)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = s.Run()
	}()
	select {
	case <-s.Listening():
	case <-done:
		panic(fmt.Sprintf("server %s: Run returned before listening", s.serviceName))
	case <-time.After(testWaitLimit):
		panic(fmt.Sprintf("server %s: not listening after %v", s.serviceName, testWaitLimit))
	}
	return done
}

// waitClosed fails loudly unless ch closes or delivers a value within
// testWaitLimit.
func waitClosed(ch <-chan struct{}, what string) {
	select {
	case <-ch:
	case <-time.After(testWaitLimit):
		panic(what + ": not done after " + testWaitLimit.String())
	}
}

// recvWithin receives from ch, failing the test if nothing arrives within
// testWaitLimit, so a step that never returns fails the test instead of
// hanging it.
func recvWithin[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(testWaitLimit):
		t.Fatalf("%s: nothing received after %v", what, testWaitLimit)
		var zero T
		return zero
	}
}

// useTestTimings shortens a client's internal waits.
func (c *Client) useTestTimings() *Client {
	c.shmAttachTimeout = testShmAttachTimeout
	return c
}

// waitHandshakesStarted waits until n workers of s are waiting for HELLO.
func waitHandshakesStarted(s *Server, n int) {
	for range n {
		select {
		case <-s.handshakeStarted:
		case <-time.After(testWaitLimit):
			panic(fmt.Sprintf("server %s: no handshake started within %v", s.serviceName, testWaitLimit))
		}
	}
}

// waitSessionExits waits until n session workers of s have exited and freed
// their slots. Only servers started by runServerForTest report exits.
func waitSessionExits(s *Server, n int) {
	for range n {
		select {
		case <-s.sessionExited:
		case <-time.After(testWaitLimit):
			panic(fmt.Sprintf("server %s: session worker did not exit within %v", s.serviceName, testWaitLimit))
		}
	}
}

// shmCreatedCount drains and counts the SHM creations s has reported. A
// server creates the objects before it sends HELLO_ACK, so every creation for
// a handshake the client finished is already reported.
func shmCreatedCount(s *Server) int {
	n := 0
	for {
		select {
		case <-s.shmCreated:
			n++
		default:
			return n
		}
	}
}
