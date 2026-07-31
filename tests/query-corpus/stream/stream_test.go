// SPDX-License-Identifier: GPL-3.0-or-later

package stream

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func startFakeParent(t *testing.T, requested uint32, response, following string, keepOpen bool) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	hold := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		defer listener.Close()
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()

		reader := bufio.NewReader(conn)
		var request strings.Builder
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				done <- fmt.Errorf("read request: %w", err)
				return
			}
			request.WriteString(line)
			if strings.HasSuffix(request.String(), "\r\n\r\n") {
				break
			}
		}
		wantVersion := "&ver=" + strconv.FormatUint(uint64(requested), 10) + "&"
		if !strings.Contains(request.String(), wantVersion) {
			done <- fmt.Errorf("request does not advertise %q", wantVersion)
			return
		}

		// One-byte writes exercise fragmented prompt/capability delivery. The
		// receiver deliberately sends no delimiter after the decimal mask.
		for _, b := range []byte(prompt + response + following) {
			if _, err := conn.Write([]byte{b}); err != nil {
				done <- nil // the client may close immediately on a deliberate mismatch
				return
			}
		}
		if keepOpen {
			<-hold
		}
		done <- nil
	}()

	var once sync.Once
	release := func() error {
		once.Do(func() { close(hold) })
		return <-done
	}
	t.Cleanup(func() {
		if err := release(); err != nil {
			t.Errorf("fake parent: %v", err)
		}
	})
	return listener.Addr().String()
}

type connectResult struct {
	conn *Conn
	err  error
}

func connectWithin(t *testing.T, addr string, requested uint32, timeout time.Duration) connectResult {
	t.Helper()

	result := make(chan connectResult, 1)
	go func() {
		conn, err := Connect(addr, "fixture-key", HostInfo{
			Hostname:    "fixture-child",
			MachineGUID: "00000000-0000-0000-0000-000000000001",
		}, requested)
		result <- connectResult{conn: conn, err: err}
	}()

	select {
	case got := <-result:
		return got
	case <-time.After(timeout):
		t.Fatalf("Connect did not finish within %s while the parent kept the undelimited reply open", timeout)
		return connectResult{}
	}
}

func TestConnectAcceptsRequiredCapabilitiesWithoutDelimiter(t *testing.T) {
	for _, requested := range []uint32{CapsLive, CapsLiveV1, CapsReplication} {
		t.Run(strconv.FormatUint(uint64(requested), 10), func(t *testing.T) {
			addr := startFakeParent(
				t, requested, strconv.FormatUint(uint64(requested), 10), "", true)
			got := connectWithin(t, addr, requested, time.Second)
			if got.err != nil {
				t.Fatal(got.err)
			}
			defer got.conn.Close()
			if got.conn.Negotiated != requested {
				t.Fatalf("negotiated capabilities = %d, want %d", got.conn.Negotiated, requested)
			}
		})
	}
}

func TestConnectRejectsDifferentCapabilityMask(t *testing.T) {
	addr := startFakeParent(
		t, CapsReplication, strconv.FormatUint(uint64(CapsLive), 10), "", true)
	got := connectWithin(t, addr, CapsReplication, time.Second)
	if got.conn != nil {
		got.conn.Close()
		t.Fatal("Connect returned a connection for a parent missing required capabilities")
	}
	if got.err == nil {
		t.Fatal("Connect accepted a parent missing required capabilities")
	}
}

func TestConnectRejectsTruncatedCapabilityMask(t *testing.T) {
	required := strconv.FormatUint(uint64(CapsReplication), 10)
	addr := startFakeParent(t, CapsReplication, required[:len(required)-1], "", false)
	got := connectWithin(t, addr, CapsReplication, time.Second)
	if got.conn != nil {
		got.conn.Close()
		t.Fatal("Connect returned a connection for a truncated capability mask")
	}
	if got.err == nil {
		t.Fatal("Connect accepted a truncated capability mask")
	}
}

func TestConnectPreservesFirstProtocolLineAfterCapabilities(t *testing.T) {
	const sentinel = "REPLAY_CHART fixture.chart 1 2"
	addr := startFakeParent(
		t, CapsReplication, strconv.FormatUint(uint64(CapsReplication), 10), sentinel+"\n", true)
	got := connectWithin(t, addr, CapsReplication, time.Second)
	if got.err != nil {
		t.Fatal(got.err)
	}
	defer got.conn.Close()

	line, err := got.conn.ReadLine(time.Now().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if line != sentinel {
		t.Fatalf("first protocol line = %q, want %q", line, sentinel)
	}
}
