// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Exercise main's actual capability wiring, file discovery and job enable path.
// The reference exceeds LAN's password limit; only resolution lets Init reach UDP.
func TestMainResolvesEnvironmentSecrets(t *testing.T) {
	if os.Getenv("NETDATA_IPMI_TEST_HELPER") == "1" {
		os.Args = []string{"ipmi.plugin", "-c", os.Getenv("NETDATA_IPMI_TEST_CONFIG"), "-j", "secret-test"}
		main()
		return
	}

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "ipmi"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "ipmi.conf"), []byte("enabled: yes\nmodules:\n  ipmi: yes\n"), 0o600))
	config := fmt.Sprintf("jobs:\n  - name: secret-test\n    driver: lan\n    hostname: 127.0.0.1\n    port: %d\n    username: monitor\n    password: '${env:IPMI_TEST_PASSWORD}'\n    timeout: 100ms\n", conn.LocalAddr().(*net.UDPAddr).Port)
	require.NoError(t, os.WriteFile(filepath.Join(root, "ipmi", "ipmi.conf"), []byte(config), 0o600))

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestMainResolvesEnvironmentSecrets$")
	cmd.Env = append(os.Environ(), "NETDATA_IPMI_TEST_HELPER=1", "NETDATA_IPMI_TEST_CONFIG="+root, "IPMI_TEST_PASSWORD=test-password")
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Start())
	scanned := make(chan struct{})
	go func() {
		defer close(scanned)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if strings.HasPrefix(scanner.Text(), "CONFIG ipmi:collector:ipmi:secret-test create ") {
				_, _ = fmt.Fprintln(stdin, `FUNCTION enable 5 "config ipmi:collector:ipmi:secret-test enable" 0xFFFF "user=test"`)
			}
		}
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Signal(os.Interrupt)
		_ = cmd.Wait()
		<-scanned
		if t.Failed() {
			t.Log(stderr.String())
		}
	})

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	buffer := make([]byte, 1024)
	_, _, err = conn.ReadFromUDP(buffer)
	require.NoError(t, err, "resolved LAN password must pass Init and reach the BMC")
}
