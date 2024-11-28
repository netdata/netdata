// SPDX-License-Identifier: GPL-3.0-or-later

package socket

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSocket_Command(t *testing.T) {
	const (
		testServerAddress     = "tcp://127.0.0.1:9999"
		testUdpServerAddress  = "udp://127.0.0.1:9999"
		testUnixServerAddress = "unix:///tmp/testSocketFD"
		defaultTimeout        = 1000 * time.Millisecond
	)

	type server interface {
		Run() error
		Close() error
	}

	tests := map[string]struct {
		srv            server
		cfg            Config
		wantConnectErr bool
		wantCommandErr bool
	}{
		"tcp": {
			srv: newTCPServer(testServerAddress),
			cfg: Config{
				Address: testServerAddress,
				Timeout: defaultTimeout,
			},
		},
		"udp": {
			srv: newUDPServer(testUdpServerAddress),
			cfg: Config{
				Address: testUdpServerAddress,
				Timeout: defaultTimeout,
			},
		},
		"unix": {
			srv: newUnixServer(testUnixServerAddress),
			cfg: Config{
				Address: testUnixServerAddress,
				Timeout: defaultTimeout,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			go func() {
				defer func() { _ = test.srv.Close() }()
				require.NoError(t, test.srv.Run())
			}()
			time.Sleep(time.Millisecond * 500)

			sock := New(test.cfg)

			err := sock.Connect()

			if test.wantConnectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			defer sock.Disconnect()

			var resp string
			err = sock.Command("ping\n", func(bytes []byte) (bool, error) {
				resp = string(bytes)
				return false, nil
			})

			if test.wantCommandErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, "pong", resp)
			}
		})
	}
}
