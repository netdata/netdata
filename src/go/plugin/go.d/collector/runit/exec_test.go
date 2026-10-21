// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package runit

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Outputs of `sv status` (runit 2.1.2) for services ok1 and ok2 running, nosup without runsv,
// and loopdown with a symlink loop as its `down` file.
const (
	svOutOK1 = "run: /svc/ok1: (pid 22) 2s\n"
	svOutOK2 = "run: /svc/ok2: (pid 24) 2s\n"
	// sv reports an unreadable `down` file on stderr and leaves an empty line on stdout.
	svOutLoopdown = "\n"
	svErrLoopdown = "warning: unable to stat /svc/loopdown/down: symbolic link loop\n"
	svOutNosup    = "warning: /svc/nosup: unable to open supervise/ok: file does not exist\n"
	// The shell passes the glob as is when the service directory is empty.
	svOutNoServices = "fail: /svc/*: unable to change to service directory: file does not exist\n"
)

func TestSvCliExec_StatusAll(t *testing.T) {
	tests := map[string]struct {
		stdout   string
		stderr   string
		finish   string // Shell code that ends the fake ndsudo.
		wantData string
		wantErr  bool
	}{
		"all services reported": {
			stdout:   svOutOK1 + svOutOK2,
			finish:   "exit 0",
			wantData: svOutOK1 + svOutOK2,
		},
		"no services": {
			stdout:   svOutNoServices,
			finish:   "exit 1",
			wantData: svOutNoServices,
		},
		"unreadable down file": {
			stdout:   svOutOK1 + svOutLoopdown + svOutOK2,
			stderr:   svErrLoopdown,
			finish:   "exit 0",
			wantData: svOutOK1 + svOutLoopdown + svOutOK2,
		},
		"service without runsv": {
			stdout:   svOutOK1 + svOutNosup + svOutOK2,
			finish:   "exit 1",
			wantData: svOutOK1 + svOutNosup + svOutOK2,
		},
		"unreadable down file and service without runsv": {
			stdout:   svOutOK1 + svOutLoopdown + svOutNosup + svOutOK2,
			stderr:   svErrLoopdown,
			finish:   "exit 1",
			wantData: svOutOK1 + svOutLoopdown + svOutNosup + svOutOK2,
		},
		"sv fatal error after partial output": {
			stdout:   svOutOK1,
			stderr:   "fatal: unable to change to original directory: permission denied\n",
			finish:   "exit 100",
			wantData: svOutOK1,
		},
		"unexpected exit code after partial output": {
			stdout:   svOutOK1,
			finish:   "exit 150",
			wantData: svOutOK1,
		},
		"timeout after partial output": {
			stdout:   svOutOK1,
			finish:   "exec sleep 10",
			wantData: svOutOK1,
		},
		"killed after partial output": {
			stdout:   svOutOK1,
			finish:   "kill -KILL $$",
			wantData: svOutOK1,
		},
		"ndsudo error": {
			stderr:  "ndsudo: failed to acquire root privileges (is the binary setuid-root?): Operation not permitted\n",
			finish:  "exit 7",
			wantErr: true,
		},
		"shell rejects option": {
			stderr:  "sh: illegal option -p\n",
			finish:  "exit 2",
			wantErr: true,
		},
		"sv not found": {
			stderr:  "sh: exec: sv: not found\n",
			finish:  "exit 127",
			wantErr: true,
		},
		"sv fatal error before output": {
			stderr:  "usage: sv [-v] [-w sec] command service ...\n",
			finish:  "exit 100",
			wantErr: true,
		},
		"timeout without output": {
			finish:  "exec sleep 10",
			wantErr: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ndsudoPath := prepareNDSudoSvStatus(t, test.stdout, test.stderr, test.finish)
			ndexec.SetRunnerPathsForTests("", ndsudoPath)
			t.Cleanup(func() {
				ndexec.SetRunnerPathsForTests("", filepath.Join(buildinfo.PluginsDir, "ndsudo"))
			})
			cli := &svCliExec{
				timeout: time.Second,
				log:     logger.NewWithWriter(io.Discard),
			}

			bs, err := cli.StatusAll(t.TempDir())

			if test.wantErr {
				assert.Error(t, err)
				assert.Nil(t, bs)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.wantData, string(bs))
			}
		})
	}
}

func prepareNDSudoSvStatus(t *testing.T, stdout, stderr, finish string) string {
	t.Helper()

	dir := t.TempDir()
	stdoutPath := filepath.Join(dir, "stdout")
	stderrPath := filepath.Join(dir, "stderr")
	require.NoError(t, os.WriteFile(stdoutPath, []byte(stdout), 0o644))
	require.NoError(t, os.WriteFile(stderrPath, []byte(stderr), 0o644))

	path := filepath.Join(dir, "ndsudo")
	script := "#!/bin/sh\ncat '" + stdoutPath + "'\ncat '" + stderrPath + "' >&2\n" + finish + "\n"
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))

	return path
}
