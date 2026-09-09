// SPDX-License-Identifier: GPL-3.0-or-later

package netlistensd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLocalListenersExec_discoverMissingHelper(t *testing.T) {
	// local-listeners is built only on Linux (ENABLE_PLUGIN_LOCAL_LISTENERS),
	// so an absent helper must be reported without ever invoking nd-run.
	e := &localListenersExec{
		binPath: filepath.Join(t.TempDir(), "local-listeners"),
		timeout: time.Second,
	}

	bs, err := e.discover(t.Context())

	require.ErrorIs(t, err, errLocalListenersNotInstalled)
	require.ErrorContains(t, err, e.binPath)
	require.Nil(t, bs)
}

func TestLocalListenersExec_discoverReportsUnexpectedStatError(t *testing.T) {
	// A path whose parent is a regular file yields ENOTDIR, not ENOENT: the
	// helper may well be installed, so this must not be reported as "not
	// installed".
	file := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	e := &localListenersExec{
		binPath: filepath.Join(file, "local-listeners"),
		timeout: time.Second,
	}

	_, err := e.discover(t.Context())

	require.Error(t, err)
	require.NotErrorIs(t, err, errLocalListenersNotInstalled)
}
