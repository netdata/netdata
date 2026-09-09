// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package fail2ban

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFail2banClientCliExec_NDSudoArgs(t *testing.T) {
	ndsudoPath := prepareNDSudoArgEcho(t)
	ndexec.SetRunnerPathsForTests("", ndsudoPath)
	t.Cleanup(func() {
		ndexec.SetRunnerPathsForTests("", filepath.Join(buildinfo.PluginsDir, "ndsudo"))
	})

	tests := map[string]struct {
		isInsideDocker bool
		call           func(*fail2banClientCliExec) ([]byte, error)
		wantArgs       []string
	}{
		"host status": {
			call: func(exec *fail2banClientCliExec) ([]byte, error) {
				return exec.status()
			},
			wantArgs: []string{"fail2ban-client-status"},
		},
		"docker status": {
			isInsideDocker: true,
			call: func(exec *fail2banClientCliExec) ([]byte, error) {
				return exec.status()
			},
			wantArgs: []string{"fail2ban-client-status-socket"},
		},
		"host jail status": {
			call: func(exec *fail2banClientCliExec) ([]byte, error) {
				return exec.jailStatus("sshd")
			},
			wantArgs: []string{"fail2ban-client-status-jail", "--jail", "sshd"},
		},
		"docker jail status": {
			isInsideDocker: true,
			call: func(exec *fail2banClientCliExec) ([]byte, error) {
				return exec.jailStatus("sshd")
			},
			wantArgs: []string{"fail2ban-client-status-jail-socket", "--jail", "sshd"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			exec := &fail2banClientCliExec{
				Logger:         logger.NewWithWriter(io.Discard),
				timeout:        time.Second,
				isInsideDocker: test.isInsideDocker,
			}

			bs, err := test.call(exec)

			require.NoError(t, err)
			assert.Equal(t, test.wantArgs, splitLines(bs))
		})
	}
}

func prepareNDSudoArgEcho(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "ndsudo")
	require.NoError(t, os.WriteFile(path, []byte(`#!/bin/sh
for arg do
  printf '%s\n' "$arg"
done
`), 0o755))

	return path
}

func splitLines(bs []byte) []string {
	s := strings.TrimSuffix(string(bs), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
