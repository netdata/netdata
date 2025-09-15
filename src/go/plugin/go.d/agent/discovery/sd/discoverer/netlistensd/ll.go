// SPDX-License-Identifier: GPL-3.0-or-later

package netlistensd

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

type localListeners interface {
	discover(ctx context.Context) ([]byte, error)
}

func newLocalListeners(timeout time.Duration) localListeners {
	dir := os.Getenv("NETDATA_PLUGINS_DIR")
	if dir == "" {
		dir = executable.Directory
	}
	if dir == "" {
		dir, _ = os.Getwd()
	}

	return &localListenersExec{
		binPath: filepath.Join(dir, "local-listeners"),
		timeout: timeout,
	}
}

type localListenersExec struct {
	binPath string
	timeout time.Duration
}

func (e *localListenersExec) discover(ctx context.Context) ([]byte, error) {
	// TCPv4/6 and UPDv4 sockets in LISTEN state
	// https://github.com/netdata/netdata/blob/master/src/collectors/utils/local_listeners.c
	args := []string{
		e.binPath,
		"no-udp6",
		"no-local",
		"no-inbound",
		"no-outbound",
		"no-namespaces",
	}

	return ndexec.RunUnprivileged(nil, e.timeout, args...)
}
