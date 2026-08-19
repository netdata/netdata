// SPDX-License-Identifier: GPL-3.0-or-later

package netlistensd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

// errLocalListenersNotInstalled means the local-listeners helper is not present.
// It is built only on Linux (ENABLE_PLUGIN_LOCAL_LISTENERS), so on every other
// platform this is the expected state, not a failure.
var errLocalListenersNotInstalled = errors.New("local-listeners helper is not installed")

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
	if _, err := os.Stat(e.binPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w ('%s')", errLocalListenersNotInstalled, e.binPath)
		}
		return nil, err
	}

	// TCPv4/6 and UPDv4 sockets in LISTEN state
	// https://github.com/netdata/netdata/blob/master/src/collectors/utils/local_listeners.c
	args := []string{
		"no-udp6",
		"no-local",
		"no-inbound",
		"no-outbound",
		"no-namespaces",
	}

	out, _, _, err := ndexec.RunUnprivilegedWithOptionsUsageContext(
		ctx,
		nil,
		e.timeout,
		ndexec.RunOptions{},
		e.binPath,
		args...,
	)
	return out, err
}
