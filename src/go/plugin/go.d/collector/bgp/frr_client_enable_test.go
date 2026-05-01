// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFRRClient_DrainsEnableResponseBeforeNextCommand(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "bgpd.vty")
	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	errCh := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)

		conn, err := ln.Accept()
		if err != nil {
			if !strings.Contains(err.Error(), "closed network connection") {
				errCh <- err
			}
			return
		}
		defer conn.Close()

		cmd, err := readNullTerminated(conn)
		if err != nil {
			errCh <- err
			return
		}
		if cmd != "enable" {
			errCh <- assert.AnError
			return
		}

		enableReply := []byte(strings.Repeat("x", 5000))
		for _, part := range [][]byte{
			enableReply[:2048],
			enableReply[2048:4096],
			append(enableReply[4096:], 0),
		} {
			if _, err := conn.Write(part); err != nil {
				errCh <- err
				return
			}
		}

		cmd, err = readNullTerminated(conn)
		if err != nil {
			errCh <- err
			return
		}
		if cmd != "show bgp vrf all ipv4 summary json" {
			errCh <- assert.AnError
			return
		}

		_, err = conn.Write(append([]byte(`{"ipv4Unicast":{"routerId":"192.0.2.254"}}`), 0))
		if err != nil {
			errCh <- err
		}
	}()

	client := &frrClient{
		socketPath: socketPath,
		timeout:    time.Second,
	}
	t.Cleanup(func() { _ = client.Close() })

	data, err := client.Summary("ipv4", "unicast")
	require.NoError(t, err)
	assert.JSONEq(t, `{"ipv4Unicast":{"routerId":"192.0.2.254"}}`, string(data))

	select {
	case err := <-errCh:
		require.NoError(t, err)
	default:
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for FRR enable replay server")
	}
}
